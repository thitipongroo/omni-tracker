import os
import json
import asyncio
import hmac
import hashlib
import base64
import httpx
import asyncpg
from fastapi import FastAPI, BackgroundTasks, Request
from urllib.parse import parse_qs
from contextlib import asynccontextmanager
from apscheduler.schedulers.asyncio import AsyncIOScheduler
import google.generativeai as genai
from pydantic import BaseModel, Field
from influxdb_client.client.influxdb_client_async import InfluxDBClientAsync
from datetime import datetime
from duckduckgo_search import AsyncDDGS
from tenacity import retry, stop_after_attempt, wait_exponential

# Environment variables
DB_URL = os.getenv("DATABASE_URL", "postgres://admin:admin@postgres:5432/omnitracker")
INFLUX_URL = os.getenv("INFLUXDB_URL", "http://influxdb:8086")
INFLUX_TOKEN = os.getenv("INFLUXDB_TOKEN", "super-secret-token")
INFLUX_ORG = os.getenv("INFLUXDB_ORG", "my-org")
INFLUX_BUCKET = os.getenv("INFLUXDB_BUCKET", "market-data")
LINE_CHANNEL_ACCESS_TOKEN = os.getenv("LINE_CHANNEL_ACCESS_TOKEN")
LINE_CHANNEL_SECRET = os.getenv("LINE_CHANNEL_SECRET")
GEMINI_API_KEY = os.getenv("GEMINI_API_KEY")

if GEMINI_API_KEY:
    genai.configure(api_key=GEMINI_API_KEY)

# Data Structures (Structured Output)
class AIResponse(BaseModel):
    product: str
    trend: str = Field(description="UP, DOWN, or STABLE")
    recommendation: str = Field(description="BUY, WAIT, or SELL")
    confidence_score: int
    reasoning: str

# Global instances for connection pooling
db_pool = None
influx_client = None
influx_query_api = None

# Database Setup (Async Connection Pooling)
async def init_services():
    global db_pool, influx_client, influx_query_api
    
    # 1. Initialize asyncpg connection pool
    try:
        db_pool = await asyncpg.create_pool(DB_URL, min_size=5, max_size=20)
        print("✅ PostgreSQL Connection Pool Initialized")
    except Exception as e:
        print(f"❌ Failed to initialize PostgreSQL pool: {e}")

    # 2. Initialize Shared Async InfluxDB Client
    try:
        influx_client = InfluxDBClientAsync(url=INFLUX_URL, token=INFLUX_TOKEN, org=INFLUX_ORG)
        influx_query_api = influx_client.query_api()
        print("✅ InfluxDB Async Client Initialized")
    except Exception as e:
        print(f"❌ Failed to initialize InfluxDB Async Client: {e}")

    # 3. Create AI Memory Table
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
        print("PostgreSQL Pool Closed")
    if influx_client:
        await influx_client.close()
        print("InfluxDB Client Closed")

# Database Operations (Async)
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
    except Exception as e:
        print(f"Error fetching last recommendation: {e}")
        return "NONE"

async def save_recommendation(line_user_id: str, product_id: str, recommendation: str):
    if not db_pool: return
    try:
        async with db_pool.acquire() as conn:
            await conn.execute("""
                INSERT INTO ai_recommendations_log (line_user_id, product_id, recommendation)
                VALUES ($1, $2, $3)
            """, line_user_id, product_id, recommendation)
    except Exception as e:
        print(f"Error saving recommendation: {e}")

async def save_feedback(line_user_id: str, product_id: str, vote: str):
    if not db_pool: return
    try:
        async with db_pool.acquire() as conn:
            await conn.execute("""
                INSERT INTO ai_feedback_log (line_user_id, product_id, vote)
                VALUES ($1, $2, $3)
            """, line_user_id, product_id, vote)
    except Exception as e:
        print(f"Error saving feedback: {e}")

async def get_users_and_products() -> dict:
    if not db_pool: return {}
    try:
        async with db_pool.acquire() as conn:
            rows = await conn.fetch("""
                SELECT u.id, u.line_user_id, p.product_id, p.store 
                FROM users u
                JOIN products p ON u.id = p.user_id
                WHERE p.is_active = true AND u.line_user_id IS NOT NULL AND u.line_user_id != ''
            """)
            users_map = {}
            for row in rows:
                line_id, prod_id, store = row['line_user_id'], row['product_id'], row['store']
                if line_id not in users_map:
                    users_map[line_id] = []
                users_map[line_id].append({"product_id": prod_id, "store": store})
            return users_map
    except Exception as e:
        print(f"Error fetching from DB: {e}")
        return {}

async def query_influxdb(product_id: str, store: str, days: int = 7) -> list:
    if not influx_query_api: return []
    query = f'''
        from(bucket: "{INFLUX_BUCKET}")
        |> range(start: -{days}d)
        |> filter(fn: (r) => r._measurement == "product_price")
        |> filter(fn: (r) => r.product_id == "{product_id}" and r.store == "{store}")
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

# Web Search (Push-based instead of Tool-based to save LLM tokens and confusion)
news_cache = {}

async def search_web(query: str) -> str:
    """Async Search using DuckDuckGo to provide real market context."""
    if query in news_cache:
        print(f"[{datetime.now()}] Using cached news for: {query}")
        return news_cache[query]
        
    print(f"[{datetime.now()}] Fetching market news for: {query}")
    try:
        # Prevent aggressive banning by adding a small delay if called concurrently
        await asyncio.sleep(1)
        async with AsyncDDGS() as ddgs:
            results = await ddgs.atext(query + " market news", max_results=2)
            if not results:
                news_cache[query] = "No recent news found."
                return news_cache[query]
            snippets = [f"- {res['title']}: {res['body']}" for res in results]
            news_cache[query] = "\n".join(snippets)
            return news_cache[query]
    except Exception as e:
        print(f"Web search error: {e}")
        return "Failed to search the web."

# LINE Messaging API
async def send_line_flex(line_user_id: str, ai_result: AIResponse):
    if not LINE_CHANNEL_ACCESS_TOKEN:
        print("Missing LINE_CHANNEL_ACCESS_TOKEN")
        return

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
                  "header": {
                    "type": "box",
                    "layout": "vertical",
                    "contents": [
                      {"type": "text", "text": "🤖 AI Market Analyst", "weight": "bold", "color": "#1DB446"}
                    ]
                  },
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
                  },
                  "footer": {
                    "type": "box",
                    "layout": "horizontal",
                    "spacing": "sm",
                    "contents": [
                      {
                        "type": "button",
                        "style": "secondary",
                        "height": "sm",
                        "action": {
                          "type": "postback",
                          "label": "👍 เห็นด้วย",
                          "data": f"action=feedback&product_id={ai_result.product}&vote=agree",
                          "displayText": f"ฉันเห็นด้วยกับการวิเคราะห์ {ai_result.product}"
                        }
                      },
                      {
                        "type": "button",
                        "style": "secondary",
                        "height": "sm",
                        "action": {
                          "type": "postback",
                          "label": "👎 ไม่เห็นด้วย",
                          "data": f"action=feedback&product_id={ai_result.product}&vote=disagree",
                          "displayText": f"ฉันไม่เห็นด้วยกับการวิเคราะห์ {ai_result.product}"
                        }
                      }
                    ]
                  }
                }
            }
        ]
    }
    async with httpx.AsyncClient() as client:
        try:
            resp = await client.post(url, headers=headers, json=payload, timeout=10.0)
            print(f"LINE Response for {line_user_id}: {resp.status_code}")
        except Exception as e:
            print(f"Failed to send LINE message: {e}")

# Data Analysis
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

# Financial Advisor (Gemini)
@retry(stop=stop_after_attempt(3), wait=wait_exponential(multiplier=1, min=2, max=10))
async def run_financial_advisor_async(product_name: str, data_stats: dict, last_rec: str, news: str) -> AIResponse:
    if not GEMINI_API_KEY: return None
        
    def _call_gemini():
        model = genai.GenerativeModel('gemini-1.5-flash')
        
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
        
        response = model.generate_content(
            prompt,
            generation_config=genai.GenerationConfig(
                response_mime_type="application/json",
                response_schema=AIResponse
            ),
            safety_settings={
                genai.types.HarmCategory.HARM_CATEGORY_HARASSMENT: genai.types.HarmBlockThreshold.BLOCK_ONLY_HIGH,
                genai.types.HarmCategory.HARM_CATEGORY_HATE_SPEECH: genai.types.HarmBlockThreshold.BLOCK_ONLY_HIGH,
                genai.types.HarmCategory.HARM_CATEGORY_SEXUALLY_EXPLICIT: genai.types.HarmBlockThreshold.BLOCK_ONLY_HIGH,
                genai.types.HarmCategory.HARM_CATEGORY_DANGEROUS_CONTENT: genai.types.HarmBlockThreshold.BLOCK_ONLY_HIGH,
            }
        )
        return response.text

    try:
        resp_text = await asyncio.to_thread(_call_gemini)
        return AIResponse.parse_raw(resp_text)
    except Exception as e:
        print(f"Failed to generate AI response: {e}")
        raise e

# Core Workflow
MAX_CONCURRENT_AI_CALLS = 3

async def process_product(line_id: str, prod_id: str, store: str, semaphore: asyncio.Semaphore):
    async with semaphore:
        # 1. Fetch DB Data concurrently
        prices = await query_influxdb(prod_id, store, days=7)
        stats = run_data_analyst(prices)
        
        if not stats or stats.get("volatility_percent", 0) < 5.0:
            return
            
        print(f"Analyzing {prod_id} (Volatility: {stats['volatility_percent']:.2f}%)")
        
        # 2. Fetch dependencies concurrently
        last_rec_task = get_last_recommendation(line_id, prod_id)
        news_task = search_web(prod_id)
        
        last_rec, news = await asyncio.gather(last_rec_task, news_task)
        
        # 3. Call AI
        try:
            ai_result = await run_financial_advisor_async(prod_id, stats, last_rec, news)
        except Exception as e:
            print(f"Fallback skipped for {product_id} due to: {e}. Failing silently to prevent spam.")
            ai_result = None
            
        if ai_result:
            await save_recommendation(line_id, prod_id, ai_result.recommendation)
            await send_line_flex(line_id, ai_result)

async def analyze_and_notify_async():
    print(f"[{datetime.now()}] Starting Automated Market Analysis...")
    news_cache.clear() # Clear cache daily for fresh news
    users = await get_users_and_products()
    
    semaphore = asyncio.Semaphore(MAX_CONCURRENT_AI_CALLS)
    tasks = []
    
    for line_id, products in users.items():
        for prod in products:
            tasks.append(process_product(line_id, prod['product_id'], prod['store'], semaphore))
            
    if tasks:
        await asyncio.gather(*tasks)
    print(f"[{datetime.now()}] Analysis complete.")

async def analyze_and_notify_with_lock():
    if not db_pool: return
    
    job_name = 'daily_ai_analysis'
    LOCK_ID = 999123
    
    async with db_pool.acquire() as conn:
        acquired = await conn.fetchval("SELECT pg_try_advisory_lock($1)", LOCK_ID)
        if not acquired:
            print(f"[{datetime.now()}] AI Analysis job is locked by another instance. Skipping.")
            return
            
        try:
            row = await conn.fetchrow("SELECT last_run FROM cron_locks WHERE job_name = $1", job_name)
            if row and row['last_run']:
                time_diff = (datetime.now() - row['last_run']).total_seconds()
                if time_diff < 43200: # 12 hours
                    print(f"[{datetime.now()}] AI Analysis job already ran recently. Skipping.")
                    return
            
            await conn.execute("""
                INSERT INTO cron_locks (job_name, last_run) 
                VALUES ($1, CURRENT_TIMESTAMP)
                ON CONFLICT (job_name) DO UPDATE SET last_run = CURRENT_TIMESTAMP
            """, job_name)
            
            await analyze_and_notify_async()
            
        finally:
            await conn.execute("SELECT pg_advisory_unlock($1)", LOCK_ID)

# Scheduler setup
scheduler = AsyncIOScheduler()
scheduler.add_job(analyze_and_notify_with_lock, 'cron', hour=8, minute=0)

# FastAPI setup
@asynccontextmanager
async def lifespan(app: FastAPI):
    await init_services()
    scheduler.start()
    yield
    scheduler.shutdown()
    await close_services()

app = FastAPI(title="Agentic AI API", lifespan=lifespan)

@app.get("/")
def health_check():
    return {"status": "ok", "db_pool": db_pool is not None, "influx_client": influx_client is not None}

@app.post("/analyze/on-demand")
async def trigger_analysis(line_user_id: str, product_id: str, store: str):
    """On-demand AI Analysis for Web Dashboard"""
    prices = await query_influxdb(product_id, store, days=7)
    stats = run_data_analyst(prices)
    if not stats:
        return {"error": "No data found"}
        
    last_rec, news = await asyncio.gather(
        get_last_recommendation(line_user_id, product_id),
        search_web(product_id)
    )
    
    try:
        ai_result = await run_financial_advisor_async(product_id, stats, last_rec, news)
    except Exception as e:
        print(f"Fallback skipped for {product_id} due to: {e}. Failing silently to prevent spam.")
        ai_result = None
        
    if ai_result:
        await save_recommendation(line_user_id, product_id, ai_result.recommendation)
        await send_line_flex(line_user_id, ai_result)
        return {"status": "success", "result": ai_result.model_dump()}
    return {"error": "AI analysis failed completely"}

@app.post("/webhook/line")
async def line_webhook(request: Request):
    """Receive webhook events from LINE Messaging API (e.g., Postback actions)"""
    signature = request.headers.get("x-line-signature")
    body_bytes = await request.body()
    
    if LINE_CHANNEL_SECRET and signature:
        hash = hmac.new(LINE_CHANNEL_SECRET.encode('utf-8'), body_bytes, hashlib.sha256).digest()
        expected_signature = base64.b64encode(hash).decode('utf-8')
        if signature != expected_signature:
            print("Webhook signature validation failed")
            return {"status": "error", "message": "Invalid signature"}

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
                        print(f"[{datetime.now()}] Feedback saved: User {line_user_id} voted '{vote}' on {product_id}")
                        
        return {"status": "ok"}
    except Exception as e:
        print(f"Webhook error: {e}")
        return {"status": "error"}
