import os
import json
import asyncio
import httpx
import psycopg2
from fastapi import FastAPI, BackgroundTasks
from contextlib import asynccontextmanager
from apscheduler.schedulers.asyncio import AsyncIOScheduler
import google.generativeai as genai
from pydantic import BaseModel, Field
from influxdb_client import InfluxDBClient
from datetime import datetime
from duckduckgo_search import DDGS
from tenacity import retry, stop_after_attempt, wait_exponential

# Environment variables
DB_URL = os.getenv("DATABASE_URL", "host=postgres user=admin password=admin dbname=omnitracker port=5432 sslmode=disable")
INFLUX_URL = os.getenv("INFLUXDB_URL", "http://influxdb:8086")
INFLUX_TOKEN = os.getenv("INFLUXDB_TOKEN", "super-secret-token")
INFLUX_ORG = os.getenv("INFLUXDB_ORG", "my-org")
INFLUX_BUCKET = os.getenv("INFLUXDB_BUCKET", "market-data")
LINE_CHANNEL_ACCESS_TOKEN = os.getenv("LINE_CHANNEL_ACCESS_TOKEN")
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

# 5. AI Memory/State (PostgreSQL)
def init_db():
    """Create table for AI memory if it doesn't exist."""
    try:
        conn = psycopg2.connect(DB_URL)
        cur = conn.cursor()
        cur.execute("""
            CREATE TABLE IF NOT EXISTS ai_recommendations_log (
                id SERIAL PRIMARY KEY,
                line_user_id VARCHAR(255),
                product_id VARCHAR(255),
                recommendation VARCHAR(50),
                created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
            )
        """)
        conn.commit()
        cur.close()
        conn.close()
    except Exception as e:
        print(f"Error initializing DB: {e}")

def get_last_recommendation(line_user_id: str, product_id: str) -> str:
    """Fetch the last recommendation for this user and product."""
    try:
        conn = psycopg2.connect(DB_URL)
        cur = conn.cursor()
        cur.execute("""
            SELECT recommendation FROM ai_recommendations_log
            WHERE line_user_id = %s AND product_id = %s
            ORDER BY created_at DESC LIMIT 1
        """, (line_user_id, product_id))
        row = cur.fetchone()
        cur.close()
        conn.close()
        return row[0] if row else "NONE"
    except Exception as e:
        print(f"Error fetching last recommendation: {e}")
        return "NONE"

def save_recommendation(line_user_id: str, product_id: str, recommendation: str):
    """Save the AI's recommendation to memory."""
    try:
        conn = psycopg2.connect(DB_URL)
        cur = conn.cursor()
        cur.execute("""
            INSERT INTO ai_recommendations_log (line_user_id, product_id, recommendation)
            VALUES (%s, %s, %s)
        """, (line_user_id, product_id, recommendation))
        conn.commit()
        cur.close()
        conn.close()
    except Exception as e:
        print(f"Error saving recommendation: {e}")

# Helper Functions
def get_users_and_products():
    """Personalization: Read users and their active products from PostgreSQL"""
    try:
        conn = psycopg2.connect(DB_URL)
        cur = conn.cursor()
        cur.execute("""
            SELECT u.id, u.line_user_id, p.product_id, p.store 
            FROM users u
            JOIN products p ON u.id = p.user_id
            WHERE p.is_active = true AND u.line_user_id IS NOT NULL AND u.line_user_id != ''
        """)
        rows = cur.fetchall()
        cur.close()
        conn.close()
        
        users_map = {}
        for row in rows:
            uid, line_id, prod_id, store = row
            if line_id not in users_map:
                users_map[line_id] = []
            users_map[line_id].append({"product_id": prod_id, "store": store})
        return users_map
    except Exception as e:
        print(f"Error fetching from DB: {e}")
        return {}

def query_influxdb(product_id: str, store: str, days: int = 7) -> list:
    """Fetch average daily prices from InfluxDB."""
    client = InfluxDBClient(url=INFLUX_URL, token=INFLUX_TOKEN, org=INFLUX_ORG)
    query_api = client.query_api()
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
        tables = query_api.query(query)
        prices = []
        for table in tables:
            for record in table.records:
                prices.append({
                    "date": record.get_time().strftime('%Y-%m-%d'),
                    "price": record.get_value()
                })
        client.close()
        return prices
    except Exception as e:
        print(f"InfluxDB error: {e}")
        return []

# 1. Real Web Search Tool (DuckDuckGo)
def search_web(query: str) -> str:
    """Tool: Search the web for actual market news or information related to a product."""
    print(f"[{datetime.now()}] Agent searching web for: {query}")
    try:
        results = DDGS().text(query, max_results=3)
        if not results:
            return "No recent news found."
        
        snippets = [f"- {res['title']}: {res['body']}" for res in results]
        return "\n".join(snippets)
    except Exception as e:
        print(f"Web search error: {e}")
        return "Failed to search the web."

# 4. Async Execution (httpx instead of requests)
async def send_line_flex(line_user_id: str, ai_result: AIResponse):
    """LINE Messaging API: Send Flex Message Asynchronously"""
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

# Multi-Agent Workflow
def run_data_analyst(prices: list) -> dict:
    """Agent 1: Data Analyst Agent"""
    if not prices:
        return {}
    
    price_vals = [p['price'] for p in prices]
    max_p = max(price_vals)
    min_p = min(price_vals)
    start_p = price_vals[0]
    end_p = price_vals[-1]
    
    volatility = 0
    if start_p > 0:
        volatility = abs(end_p - start_p) / start_p * 100
        
    return {
        "max_price": max_p,
        "min_price": min_p,
        "current_price": end_p,
        "volatility_percent": volatility,
        "prices_history": prices
    }

# 2. Redundancy Fix (1 call for tools + JSON) & 4. Rate Limiting Fix (Retry)
@retry(stop=stop_after_attempt(3), wait=wait_exponential(multiplier=1, min=2, max=10))
async def run_financial_advisor_async(product_name: str, data_stats: dict, last_recommendation: str) -> AIResponse:
    """Agent 2: Financial Advisor Agent (Uses Web Tool & Outputs JSON natively)"""
    if not GEMINI_API_KEY:
        return None
        
    def _call_gemini():
        model = genai.GenerativeModel(
            model_name='gemini-2.5-flash',
            tools=[search_web] # 3. Removed query_influxdb from tools to avoid LLM Confusion
        )
        
        prompt = f"""
        You are an expert Financial Advisor Agent. 
        Analyze the market data for: {product_name}
        Data Statistics: {json.dumps(data_stats)}
        
        IMPORTANT CONTEXT: Your last recommendation to the user for this product was "{last_recommendation}". 
        If your new recommendation is the same, acknowledge it or justify why they should still hold/wait/buy.
        
        Use the `search_web` tool to find recent news about {product_name} that might explain price changes.
        Provide a recommendation (BUY, WAIT, SELL).
        """
        
        response = model.generate_content(
            prompt,
            generation_config=genai.GenerationConfig(
                response_mime_type="application/json",
                response_schema=AIResponse
            )
        )
        return response.text

    try:
        resp_text = await asyncio.to_thread(_call_gemini)
        return AIResponse.parse_raw(resp_text)
    except Exception as e:
        print(f"Failed to generate or parse AI response: {e}")
        raise e # Let tenacity retry

# Concurrency Control (Semaphore)
MAX_CONCURRENT_AI_CALLS = 3

async def process_product(line_id: str, prod_id: str, store: str, semaphore: asyncio.Semaphore):
    """Process a single product asynchronously with concurrency limits."""
    async with semaphore:
        prices = query_influxdb(prod_id, store, days=7)
        stats = run_data_analyst(prices)
        
        if not stats:
            return
            
        if stats.get("volatility_percent", 0) < 5.0:
            print(f"Skipping {prod_id} for user {line_id} - Volatility < 5%")
            return
            
        print(f"Analyzing {prod_id} (Volatility: {stats['volatility_percent']:.2f}%)")
        
        last_rec = get_last_recommendation(line_id, prod_id)
        
        try:
            ai_result = await run_financial_advisor_async(prod_id, stats, last_rec)
            
            if ai_result:
                save_recommendation(line_id, prod_id, ai_result.recommendation)
                await send_line_flex(line_id, ai_result)
        except Exception as e:
            print(f"Failed processing {prod_id} for {line_id} after retries: {e}")

async def analyze_and_notify_async():
    """Main asynchronous function executed by the Scheduler"""
    print(f"[{datetime.now()}] Starting Automated Market Analysis...")
    init_db()
    users = get_users_and_products()
    
    semaphore = asyncio.Semaphore(MAX_CONCURRENT_AI_CALLS)
    tasks = []
    
    for line_id, products in users.items():
        for prod in products:
            tasks.append(process_product(line_id, prod['product_id'], prod['store'], semaphore))
            
    if tasks:
        await asyncio.gather(*tasks)
    print(f"[{datetime.now()}] Analysis complete.")

# Scheduler setup
scheduler = AsyncIOScheduler()
# Run every morning at 8:00 AM
scheduler.add_job(analyze_and_notify_async, 'cron', hour=8, minute=0)

# FastAPI setup (API Endpoint)
@asynccontextmanager
async def lifespan(app: FastAPI):
    init_db()
    scheduler.start()
    yield
    scheduler.shutdown()

app = FastAPI(title="Agentic AI API", lifespan=lifespan)

@app.get("/")
def health_check():
    return {"status": "ok"}

@app.post("/analyze/on-demand")
async def trigger_analysis(line_user_id: str, product_id: str, store: str):
    """On-demand AI Analysis for Web Dashboard"""
    init_db()
    prices = query_influxdb(product_id, store, days=7)
    stats = run_data_analyst(prices)
    if not stats:
        return {"error": "No data found"}
        
    last_rec = get_last_recommendation(line_user_id, product_id)
    try:
        ai_result = await run_financial_advisor_async(product_id, stats, last_rec)
        
        if ai_result:
            save_recommendation(line_user_id, product_id, ai_result.recommendation)
            await send_line_flex(line_user_id, ai_result)
            return {"status": "success", "result": ai_result.model_dump()}
        return {"error": "AI analysis failed"}
    except Exception as e:
        return {"error": f"AI analysis failed after retries: {e}"}
