import os
import requests
import google.generativeai as genai
from influxdb_client import InfluxDBClient

# Setup Environment Variables
INFLUX_URL = os.getenv("INFLUXDB_URL", "http://localhost:8086")
INFLUX_TOKEN = os.getenv("INFLUXDB_TOKEN", "super-secret-token")
INFLUX_ORG = os.getenv("INFLUXDB_ORG", "my-org")
INFLUX_BUCKET = os.getenv("INFLUXDB_BUCKET", "market-data")
LINE_NOTIFY_TOKEN = os.getenv("LINE_NOTIFY_TOKEN")
GEMINI_API_KEY = os.getenv("GEMINI_API_KEY")

def get_weekly_data():
    client = InfluxDBClient(url=INFLUX_URL, token=INFLUX_TOKEN, org=INFLUX_ORG)
    query_api = client.query_api()
    
    # Query price over the last 7 days, get daily mean
    query = f'''
        from(bucket: "{INFLUX_BUCKET}")
        |> range(start: -7d)
        |> filter(fn: (r) => r._measurement == "product_price")
        |> filter(fn: (r) => r._field == "price")
        |> aggregateWindow(every: 1d, fn: mean, createEmpty: false)
        |> yield(name: "mean")
    '''
    
    try:
        tables = query_api.query(query)
    except Exception as e:
        print(f"Error querying InfluxDB: {e}")
        return {}
    
    data_summary = {}
    for table in tables:
        for record in table.records:
            product = record.values.get('product_id')
            store = record.values.get('store')
            price = record.get_value()
            date = record.get_time().strftime('%Y-%m-%d')
            
            key = f"{product} ({store})"
            if key not in data_summary:
                data_summary[key] = []
            
            data_summary[key].append(f"{date}: {price:.2f} THB")
            
    client.close()
    return data_summary

def analyze_with_gemini(data):
    if not GEMINI_API_KEY:
        print("GEMINI_API_KEY not set in environment variables.")
        return None
        
    genai.configure(api_key=GEMINI_API_KEY)
    
    model = genai.GenerativeModel('gemini-2.5-flash')
    
    data_str = ""
    for product, prices in data.items():
        data_str += f"\n📦 สินค้า: {product}\n"
        data_str += "\n".join(prices) + "\n"
        
    prompt = f"""
คุณคือ Market Analyst ผู้เชี่ยวชาญด้านราคาสินค้า 
นี่คือข้อมูลราคาสินค้าเฉลี่ยรายวันในรอบ 7 วันที่ผ่านมา:
{data_str}

ช่วยวิเคราะห์และสรุปแนวโน้มราคาสินค้าแต่ละตัวให้หน่อย 
และแนะนำอย่างฟันธงว่าควร "ซื้อเลย" หรือ "รอไปก่อน" (เช่น ราคาต่ำสุดในรอบสัปดาห์แล้ว หรือกำลังเป็นขาลง)
ขอสั้นๆ กระชับ อ่านง่าย เหมาะสำหรับส่งเข้าแชท Line 
"""
    
    try:
        response = model.generate_content(prompt)
        return response.text
    except Exception as e:
        print(f"Error calling Gemini API: {e}")
        return None

def send_line_notify(message):
    if not LINE_NOTIFY_TOKEN:
        print("LINE_NOTIFY_TOKEN not set in environment variables.")
        return
        
    url = 'https://notify-api.line.me/api/notify'
    headers = {'Authorization': f'Bearer {LINE_NOTIFY_TOKEN}'}
    # Add a small prefix to denote AI
    data = {'message': f"\n🤖 [AI Market Analyst]\n{message}"}
    
    response = requests.post(url, headers=headers, data=data)
    if response.status_code == 200:
        print("Line message sent successfully!")
    else:
        print(f"Failed to send line message: {response.status_code} {response.text}")

if __name__ == "__main__":
    print("Fetching data from InfluxDB...")
    weekly_data = get_weekly_data()
    
    if not weekly_data:
        print("No data found for the past week.")
        exit()
        
    print("Analyzing data with Gemini...")
    analysis_result = analyze_with_gemini(weekly_data)
    
    if analysis_result:
        print("\n--- AI Result ---")
        print(analysis_result)
        print("-----------------\n")
        
        print("Sending analysis to Line...")
        send_line_notify(analysis_result)
        print("Done!")
