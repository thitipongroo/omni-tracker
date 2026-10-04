import os
import json
import asyncio
import hmac
import hashlib
import base64
import httpx
import asyncpg
import re
from fastapi import FastAPI, BackgroundTasks, Request, HTTPException, Depends, Header
from urllib.parse import parse_qs
from contextlib import asynccontextmanager
from apscheduler.schedulers.asyncio import AsyncIOScheduler
from google import genai
from pydantic import BaseModel, Field
from influxdb_client.client.influxdb_client_async import InfluxDBClientAsync
from datetime import datetime
from duckduckgo_search import AsyncDDGS
from tenacity import retry, stop_after_attempt, wait_exponential

# Environment variables
DB_URL = os.getenv("DATABASE_URL", "postgres://admin:admin@postgres:5432/omnitracker")
INFLUX_URL = os.getenv("INFLUXDB_URL", "http://influxdb:8086")
INFLUX_TOKEN = os.getenv("INFLUXDB_TOKEN")
INFLUX_ORG = os.getenv("INFLUXDB_ORG", "my-org")
INFLUX_BUCKET = os.getenv("INFLUXDB_BUCKET", "market-data")
LINE_CHANNEL_ACCESS_TOKEN = os.getenv("LINE_CHANNEL_ACCESS_TOKEN")
LINE_CHANNEL_SECRET = os.getenv("LINE_CHANNEL_SECRET")
GEMINI_API_KEY = os.getenv("GEMINI_API_KEY")
API_KEY = os.getenv("API_KEY")

gemini_client = None
if GEMINI_API_KEY:
    gemini_client = genai.Client(api_key=GEMINI_API_KEY)

class AIResponse(BaseModel):
    product: str
    trend: str = Field(description="UP, DOWN, or STABLE")
    recommendation: str = Field(description="BUY, WAIT, or SELL")
    confidence_score: int
    reasoning: str

db_pool = None
influx_client = None
influx_query_api = None

async def init_services():
    global db_pool, influx_client, influx_query_api
    
    for i in range(10):
        try:
            db_pool = await asyncpg.create_pool(DB_URL, min_size=5, max_size=20)
            print("✅ PostgreSQL Connection Pool Initialized")
            break
        except Exception as e:
            print(f"⏳ Waiting for PostgreSQL: {e}")
            await asyncio.sleep(3)

    try:
        influx_client = InfluxDBClientAsync(url=INFLUX_URL, token=INFLUX_TOKEN, org=INFLUX_ORG)
        influx_query_api = influx_client.query_api()
        print("✅ InfluxDB Async Client Initialized")
    except Exception as e:
        print(f"❌ Failed to initialize InfluxDB Async Client: {e}")

    if db_pool:
        try:
            async with db_pool.acquire() as conn:
                await conn.execute("""
                    CREATE TABLE IF NOT EXISTS ai_recommendations_log (
                        id SERIAL PRIMARY KEY,
                        line_user_id VARCHAR(255),
                        product_id VARCHAR(255),
                        recommendation VARCHAR(50),
                        created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
                    )
                """)
                await conn.execute("""
                    CREATE TABLE IF NOT EXISTS ai_feedback_log (
                        id SERIAL PRIMARY KEY,
                        line_user_id VARCHAR(255),
                        product_id VARCHAR(255),
                        vote VARCHAR(20),
                        created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
                    )
                """)
                await conn.execute("""
                    CREATE TABLE IF NOT EXISTS cron_locks (
                        job_name VARCHAR(50) PRIMARY KEY,
                        last_run TIMESTAMP
                    )
                """)
        except Exception as e:
            print(f"Error creating tables: {e}")

async def close_services():
    if db_pool:
        await db_pool.close()
    if influx_client:
        await influx_client.close()

async def get_last_recommendation(line_user_id: str, product_id: str) -> str:
    if not db_pool: return "NONE"
    try:
        async with db_pool.acquire() as conn:
            row = await conn.fetchrow("""
                SELECT recommendation FROM ai_recommendations_log
                WHERE line_user_id = $1 AND product_id = $2
                ORDER BY created_at DESC LIMIT 1
            """, line_user_id, product_id)
            return row['recommendation'] if row else "NONE"
    except Exception:
        return "NONE"

async def save_recommendation(line_user_id: str, product_id: str, recommendation: str):
    if not db_pool: return
    try:
        async with db_pool.acquire() as conn:
            await conn.execute("""
                INSERT INTO ai_recommendations_log (line_user_id, product_id, recommendation)
                VALUES ($1, $2, $3)
            """, line_user_id, product_id, recommendation)
    except Exception:
        pass

async def save_feedback(line_user_id: str, product_id: str, vote: str):
    if not db_pool: return
    try:
        async with db_pool.acquire() as conn:
            await conn.execute("""
                INSERT INTO ai_feedback_log (line_user_id, product_id, vote)
                VALUES ($1, $2, $3)
            """, line_user_id, product_id, vote)
    except Exception:
        pass

async def get_users_and_products() -> dict:
    if not db_pool: return {}
    try:
        async with db_pool.acquire() as conn:
            rows = await conn.fetch("""
                SELECT u.id, u.line_user_id, p.id as product_pk, p.product_id, p.store 
                FROM users u
                JOIN products p ON u.id = p.user_id
                WHERE p.is_active = true AND u.line_user_id IS NOT NULL AND u.line_user_id != ''
            """)
            users_map = {}
            for row in rows:
                line_id, pk, prod_id, store = row['line_user_id'], row['product_pk'], row['product_id'], row['store']
                if line_id not in users_map:
                    users_map[line_id] = []
                users_map[line_id].append({"product_pk": str(pk), "product_id": prod_id, "store": store})
            return users_map
    except Exception as e:
        print(f"Error fetching from DB: {e}")
        return {}

async def query_influxdb(product_pk: str, store: str, days: int = 7) -> list:
    if not influx_query_api: return []
    # Anti-Injection
    if not re.match(r'^\d+$', str(product_pk)) or not re.match(r'^[a-z]+$', store):
        return []
        
    query = f'''
        from(bucket: "{INFLUX_BUCKET}")
        |> range(start: -{days}d)
        |> filter(fn: (r) => r._measurement == "product_price")
        |> filter(fn: (r) => r.product_pk == "{product_pk}" and r.store == "{store}")
        |> filter(fn: (r) => r._field == "price")
        |> aggregateWindow(every: 1d, fn: mean, createEmpty: false)
        |> yield(name: "mean")
    '''
    try:
        tables = await influx_query_api.query(query)
        prices = []
        for table in tables:
            for record in table.records:
                prices.append({
                    "date": record.get_time().strftime('%Y-%m-%d'),
                    "price": record.get_value()
                })
        return prices
    except Exception as e:
        print(f"InfluxDB error: {e}")
        return []

news_cache = {}

async def search_web(query: str) -> str:
    if query in news_cache:
        return news_cache[query]
        
    try:
        await asyncio.sleep(1)
        async with AsyncDDGS() as ddgs:
            results = await ddgs.text(query + " market news", max_results=2)
            if not results:
                news_cache[query] = "No recent news found."
                return news_cache[query]
            snippets = [f"- {res['title']}: {res['body']}" for res in results]
            news_cache[query] = "\n".join(snippets)
            return news_cache[query]
    except Exception as e:
        return "Failed to search the web."

async def send_line_flex(line_user_id: str, ai_result: AIResponse):
    if not LINE_CHANNEL_ACCESS_TOKEN: return

    url = 'https://api.line.me/v2/bot/message/push'
    headers = {
        'Content-Type': 'application/json',
        'Authorization': f'Bearer {LINE_CHANNEL_ACCESS_TOKEN}'
    }
    
    color = "#00B900" if ai_result.recommendation == "BUY" else "#FF334B"
    if ai_result.recommendation == "WAIT":
        color = "#F0AD4E"
    
    payload = {
        "to": line_user_id,
        "messages": [
            {
                "type": "flex",
                "altText": f"AI Market Analysis: {ai_result.product}",
                "contents": {
                  "type": "bubble",
                  "body": {
                    "type": "box",
                    "layout": "vertical",
                    "contents": [
                      {"type": "text", "text": ai_result.product, "weight": "bold", "size": "xl", "wrap": True},
                      {"type": "text", "text": f"Trend: {ai_result.trend}", "color": "#888888", "margin": "sm"},
                      {"type": "text", "text": f"Recommend: {ai_result.recommendation}", "weight": "bold", "color": color, "size": "lg", "margin": "sm"},
                      {"type": "text", "text": f"Confidence: {ai_result.confidence_score}%", "size": "sm", "margin": "sm"},
                      {"type": "text", "text": ai_result.reasoning, "wrap": True, "margin": "md", "size": "sm"}
                    ]
                  }
                }
            }
        ]
    }
    async with httpx.AsyncClient() as client:
        try:
            await client.post(url, headers=headers, json=payload, timeout=10.0)
        except Exception:
            pass

def run_data_analyst(prices: list) -> dict:
    if not prices: return {}
    price_vals = [p['price'] for p in prices]
    start_p, end_p = price_vals[0], price_vals[-1]
    
    volatility = abs(end_p - start_p) / start_p * 100 if start_p > 0 else 0
        
    return {
        "max_price": max(price_vals),
        "min_price": min(price_vals),
        "current_price": end_p,
        "volatility_percent": volatility,
        "prices_history": prices
    }

@retry(stop=stop_after_attempt(3), wait=wait_exponential(multiplier=1, min=2, max=10))
async def run_financial_advisor_async(product_name: str, data_stats: dict, last_rec: str, news: str) -> AIResponse:
    if not gemini_client: return None
        
    def _call_gemini():
        prompt = f"""
        You are an expert Financial Advisor Agent. 
        Analyze the market data for: {product_name}
        
        Data Statistics: {json.dumps(data_stats)}
        
        Market News Context:
        {news}
        
        IMPORTANT: Your last recommendation to the user for this product was "{last_rec}". 
        If your new recommendation is the same, acknowledge it or justify why they should still hold/wait/buy.
        
        Provide a recommendation (BUY, WAIT, SELL) based on the price data and news.
        """
        
        response = gemini_client.models.generate_content(
            model='gemini-2.5-flash',
            contents=prompt,
            config=genai.types.GenerateContentConfig(
                response_mime_type="application/json",
                response_schema=AIResponse,
                temperature=0.7,
            )
        )
        return response.text

    try:
        resp_text = await asyncio.to_thread(_call_gemini)
        return AIResponse.model_validate_json(resp_text)
    except Exception as e:
        print(f"Failed to generate AI response: {e}")
        raise e

MAX_CONCURRENT_AI_CALLS = 3

async def process_product(line_id: str, prod_pk: str, prod_id: str, store: str, semaphore: asyncio.Semaphore):
    async with semaphore:
        prices = await query_influxdb(prod_pk, store, days=7)
        stats = run_data_analyst(prices)
        
        if not stats or stats.get("volatility_percent", 0) < 5.0:
            return
            
        last_rec_task = get_last_recommendation(line_id, prod_id)
        news_task = search_web(prod_id)
        
        last_rec, news = await asyncio.gather(last_rec_task, news_task)
        
        try:
            ai_result = await run_financial_advisor_async(prod_id, stats, last_rec, news)
        except Exception:
            ai_result = None
            
        if ai_result:
            await save_recommendation(line_id, prod_id, ai_result.recommendation)
            await send_line_flex(line_id, ai_result)

async def analyze_and_notify_async():
    news_cache.clear()
    users = await get_users_and_products()
    
    semaphore = asyncio.Semaphore(MAX_CONCURRENT_AI_CALLS)
    tasks = []
    
    for line_id, products in users.items():
        for prod in products:
            tasks.append(process_product(line_id, prod['product_pk'], prod['product_id'], prod['store'], semaphore))
            
    if tasks:
        await asyncio.gather(*tasks)

async def analyze_and_notify_with_lock():
    if not db_pool: return
    
    job_name = 'daily_ai_analysis'
    LOCK_ID = 999123
    
    async with db_pool.acquire() as conn:
        acquired = await conn.fetchval("SELECT pg_try_advisory_lock($1)", LOCK_ID)
        if not acquired: return
            
        try:
            row = await conn.fetchrow("SELECT EXTRACT(EPOCH FROM (CURRENT_TIMESTAMP - last_run)) AS time_diff FROM cron_locks WHERE job_name = $1", job_name)
            if row and row['time_diff'] is not None:
                if float(row['time_diff']) < 43200:
                    return
            
            await conn.execute("""
                INSERT INTO cron_locks (job_name, last_run) 
                VALUES ($1, CURRENT_TIMESTAMP)
                ON CONFLICT (job_name) DO UPDATE SET last_run = CURRENT_TIMESTAMP
            """, job_name)
            
            await analyze_and_notify_async()
            
        finally:
            await conn.execute("SELECT pg_advisory_unlock($1)", LOCK_ID)

scheduler = AsyncIOScheduler()
scheduler.add_job(analyze_and_notify_with_lock, 'cron', hour=8, minute=0)

@asynccontextmanager
async def lifespan(app: FastAPI):
    await init_services()
    scheduler.start()
    yield
    scheduler.shutdown()
    await close_services()

app = FastAPI(title="Agentic AI API", lifespan=lifespan)

def verify_api_key(authorization: str = Header(None)):
    if not authorization or not authorization.startswith("Bearer "):
        raise HTTPException(status_code=401, detail="Missing API Key")
    token = authorization.split(" ")[1]
    if not hmac.compare_digest(token, API_KEY or ""):
        raise HTTPException(status_code=401, detail="Invalid API Key")

@app.post("/analyze/on-demand", dependencies=[Depends(verify_api_key)])
async def trigger_analysis(line_user_id: str, product_pk: str, product_id: str, store: str):
    prices = await query_influxdb(product_pk, store, days=7)
    stats = run_data_analyst(prices)
    if not stats:
        return {"error": "No data found"}
        
    last_rec, news = await asyncio.gather(
        get_last_recommendation(line_user_id, product_id),
        search_web(product_id)
    )
    
    try:
        ai_result = await run_financial_advisor_async(product_id, stats, last_rec, news)
    except Exception:
        ai_result = None
        
    if ai_result:
        await save_recommendation(line_user_id, product_id, ai_result.recommendation)
        await send_line_flex(line_user_id, ai_result)
        return {"status": "success", "result": ai_result.model_dump()}
    return {"error": "AI analysis failed"}

@app.post("/webhook/line")
async def line_webhook(request: Request):
    signature = request.headers.get("x-line-signature")
    body_bytes = await request.body()
    
    if not LINE_CHANNEL_SECRET or not signature:
        raise HTTPException(status_code=401, detail="Missing signature")
        
    hash_val = hmac.new(LINE_CHANNEL_SECRET.encode('utf-8'), body_bytes, hashlib.sha256).digest()
    expected_signature = base64.b64encode(hash_val).decode('utf-8')
    
    if not hmac.compare_digest(signature, expected_signature):
        raise HTTPException(status_code=401, detail="Invalid signature")

    try:
        body = json.loads(body_bytes.decode('utf-8'))
        events = body.get("events", [])
        
        for event in events:
            if event.get("type") == "postback":
                line_user_id = event.get("source", {}).get("userId")
                postback_data = event.get("postback", {}).get("data", "")
                
                parsed = parse_qs(postback_data)
                if parsed.get("action", [""])[0] == "feedback":
                    product_id = parsed.get("product_id", [""])[0]
                    vote = parsed.get("vote", [""])[0]
                    
                    if line_user_id and product_id and vote:
                        await save_feedback(line_user_id, product_id, vote)
        return {"status": "ok"}
    except Exception as e:
        return {"status": "error", "message": str(e)}
