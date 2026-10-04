import os
import json
import requests
import psycopg2
from fastapi import FastAPI, BackgroundTasks
from contextlib import asynccontextmanager
from apscheduler.schedulers.background import BackgroundScheduler
import google.generativeai as genai
from pydantic import BaseModel, Field
from influxdb_client import InfluxDBClient
from datetime import datetime

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
    """Tool: Fetch average daily prices from InfluxDB."""
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

def search_web(query: str) -> str:
    """Tool: Search the web for news or information related to a product."""
    print(f"Agent called search_web for: {query}")
    # Simulating a web search response for demonstration purposes
    return f"Latest news for '{query}': Analysts predict a new model release soon, which may affect current prices."

def send_line_flex(line_user_id: str, ai_result: AIResponse):
    """LINE Messaging API: Send Flex Message"""
    if not LINE_CHANNEL_ACCESS_TOKEN:
        print("Missing LINE_CHANNEL_ACCESS_TOKEN")
        return

    url = 'https://api.line.me/v2/bot/message/push'
    headers = {
        'Content-Type': 'application/json',
        'Authorization': f'Bearer {LINE_CHANNEL_ACCESS_TOKEN}'
    }
    
    color = "#00B900" if ai_result.recommendation == "BUY" else "#FF334B"
    
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
    resp = requests.post(url, headers=headers, json=payload)
    print(f"LINE Response: {resp.status_code}")

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

def run_financial_advisor(product_name: str, data_stats: dict) -> AIResponse:
    """Agent 2: Financial Advisor Agent (Uses Tools & Outputs JSON)"""
    if not GEMINI_API_KEY:
        return None
        
    model = genai.GenerativeModel(
        model_name='gemini-2.5-flash',
        tools=[search_web, query_influxdb] # Providing tools to Agent
    )
    
    prompt = f"""
    You are an expert Financial Advisor Agent. 
    Analyze the market data for: {product_name}
    Data Statistics: {json.dumps(data_stats)}
    
    Use the `search_web` tool if you need context on why the price dropped or surged.
    Based on your analysis, provide a recommendation.
    """
    
    # 1. Ask the model to analyze (it might use tools here)
    response = model.generate_content(prompt)
    
    # 2. Force Structured Output (JSON)
    json_model = genai.GenerativeModel('gemini-2.5-flash')
    structured_prompt = f"Extract the final recommendation for {product_name} based on this context: {response.text}\nData: {json.dumps(data_stats)}"
    
    json_resp = json_model.generate_content(
        structured_prompt,
        generation_config=genai.GenerationConfig(
            response_mime_type="application/json",
            response_schema=AIResponse
        )
    )
    
    try:
        return AIResponse.parse_raw(json_resp.text)
    except Exception as e:
        print(f"Failed to parse JSON: {e}")
        return None

def analyze_and_notify():
    """Main function executed by the Scheduler"""
    print(f"[{datetime.now()}] Starting Automated Market Analysis...")
    users = get_users_and_products()
    
    for line_id, products in users.items():
        for prod in products:
            prod_id = prod['product_id']
            store = prod['store']
            
            prices = query_influxdb(prod_id, store, days=7)
            stats = run_data_analyst(prices)
            
            if not stats:
                continue
                
            # Context Window Limit: Filter low volatility
            if stats.get("volatility_percent", 0) < 5.0:
                print(f"Skipping {prod_id} for user {line_id} - Volatility < 5%")
                continue
                
            print(f"Analyzing {prod_id} (Volatility: {stats['volatility_percent']:.2f}%)")
            ai_result = run_financial_advisor(prod_id, stats)
            
            if ai_result:
                send_line_flex(line_id, ai_result)

# Scheduler
scheduler = BackgroundScheduler()
# Run every morning at 8:00 AM
scheduler.add_job(analyze_and_notify, 'cron', hour=8, minute=0)

# FastAPI setup (API Endpoint)
@asynccontextmanager
async def lifespan(app: FastAPI):
    scheduler.start()
    yield
    scheduler.shutdown()

app = FastAPI(title="Agentic AI API", lifespan=lifespan)

@app.get("/")
def health_check():
    return {"status": "ok"}

@app.post("/analyze/on-demand")
def trigger_analysis(line_user_id: str, product_id: str, store: str):
    """On-demand AI Analysis for Web Dashboard"""
    prices = query_influxdb(product_id, store, days=7)
    stats = run_data_analyst(prices)
    if not stats:
        return {"error": "No data found"}
        
    ai_result = run_financial_advisor(product_id, stats)
    if ai_result:
        send_line_flex(line_user_id, ai_result)
        return {"status": "success", "result": ai_result.model_dump()}
    return {"error": "AI analysis failed"}
