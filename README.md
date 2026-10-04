# 🚀 Omni-Tracker

**Omni-Tracker** is an enterprise-grade, microservice-based e-commerce price tracking and AI-driven market analysis platform. It autonomously monitors product prices across multiple marketplaces (e.g., Shopee, Lazada), detects anomalies, alerts users in real-time, and leverages Google Gemini AI to provide actionable investment and purchasing recommendations via LINE Flex Messages.

---

## ✨ Core Features

* 🔐 **Multi-Tenant Security:** Secure JWT-based authentication and user registration system.
* 🕷️ **Evasive Scraper Fleet:** Playwright-based Node.js workers with Proxy Rotation, API interception, and DOM parsing fallback. Hardened with strict timeouts to prevent zombie processes.
* 🤖 **AI Market Analyst:** Python (FastAPI) service powered by **Google Gemini 1.5 Flash**. Analyzes market volatility, web news context, and historical price data to deliver `BUY/WAIT/SELL` insights.
* 📊 **Time-Series Data:** High-performance data ingestion using **InfluxDB** for storing massive price fluctuation histories.
* 💬 **Interactive LINE Integration:** Delivers rich LINE Flex Messages with dynamic feedback voting mechanisms (Agree/Disagree) directly within the LINE app.
* 🐳 **Production-Ready Dockerization:** Fully containerized with a Multi-stage Docker build, zero-downtime queue architecture, and timezone-synchronized containers.

---

## 🏗️ Architecture overview

The system is fully decoupled into **3 primary microservices** and **3 infrastructure databases**:

```mermaid
graph TD;
    User[User/Browser] -->|React SPA| GoAPI[Go Fiber API];
    User -->|LINE App| Webhook[Python AI Analyst];
    
    GoAPI -->|Write Tasks| Redis[(Redis Queue)];
    GoAPI -->|Manage Users/Products| PG[(PostgreSQL)];
    GoAPI -->|Write/Query Prices| Influx[(InfluxDB)];
    
    Scraper[Node.js Scraper Worker] -->|Pop Tasks| Redis;
    Scraper -->|Scrape| Store(Shopee / Lazada);
    Scraper -->|Send Price/Status| GoAPI;

    Python[Python AI Analyst] -->|Read Products| PG;
    Python -->|Read Prices| Influx;
    Python -->|Analyze| Gemini(Google Gemini AI);
    Python -->|Push Flex Msg| LINE(LINE Messaging API);
```

### 1. Go API (Core Gateway)
* Built with **Go Fiber**.
* Handles User Auth, rate limiting, and RESTful operations.
* Contains an internal Scheduler that prevents **Queue Poisoning** by safely pushing tasks to Redis.
* Serves the React Dashboard statically with **SPA fallback routing**.

### 2. Node.js Scraper Worker
* Built with **Playwright-extra** and `puppeteer-stealth`.
* Consumes tasks from Redis (`brpop`).
* intercepts XHR API responses for maximum speed, falling back to DOM parsing if needed.

### 3. Python AI Analyst
* Built with **FastAPI** and `APScheduler`.
* Fetches time-series data from InfluxDB and performs volatility calculations.
* Aggregates real-time news via DuckDuckGo and analyzes data using Gemini.
* Safe timezone locking mechanism using PostgreSQL `EXTRACT(EPOCH)`.

---

## 🚀 Quick Start (Docker Compose)

### 1. Prerequisites
Ensure you have the following installed:
* [Docker Desktop](https://www.docker.com/products/docker-desktop/) or Docker Engine
* Docker Compose (v2+)

### 2. Configuration
Create a `.env` file in the root directory and configure the following variables:

```env
# Core System
API_PORT=3000
API_KEY=your-internal-secure-key-123
JWT_SECRET=your-jwt-secret-key
ADMIN_PASSWORD=admin123

# Databases
DATABASE_URL=host=postgres user=admin password=admin dbname=omnitracker port=5432 sslmode=disable
REDIS_URL=redis://redis:6379
INFLUXDB_URL=http://influxdb:8086
INFLUXDB_TOKEN=super-secret-token
INFLUXDB_ORG=my-org
INFLUXDB_BUCKET=market-data

# External APIs
LINE_NOTIFY_TOKEN=your_line_notify_token (Optional)
LINE_CHANNEL_ACCESS_TOKEN=your_line_channel_access_token
LINE_CHANNEL_SECRET=your_line_channel_secret
GEMINI_API_KEY=your_google_gemini_api_key

# Scraper Configuration
PROXY_URL=http://user:pass@proxy.example.com:8080 (Optional)
```

### 3. Build & Run
Simply run the following command to spin up the entire cluster:
```bash
docker-compose up --build -d
```

### 4. Access the Platform
* **Dashboard / API:** [http://localhost:3000](http://localhost:3000)
* **AI Analyst Webhook:** `http://localhost:8000/webhook/line`
* **InfluxDB Admin UI:** [http://localhost:8086](http://localhost:8086)

---

## 📡 API Endpoints Reference

### Public Routes (Go API)
| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/api/login` | Authenticate user and receive JWT token. |
| `POST` | `/api/register` | Register a new user account. |

### Protected Routes (Requires `Authorization: Bearer <JWT>`)
| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET`  | `/api/profile` | Get current user profile and LINE ID. |
| `POST` | `/api/profile/line` | Link LINE User ID to account. |
| `GET`  | `/api/products` | List all tracked products for the user. |
| `POST` | `/api/products` | Add a new product to track. |
| `DELETE`| `/api/products/:id` | Stop tracking and delete a product. |
| `GET`  | `/api/history/:product_id` | Get 30-day time-series price history. |
| `GET`  | `/api/logs` | View recent system logs for user's products. |

### Internal / Scraper Routes (Requires `Authorization: Bearer <API_KEY>`)
| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/api/prices` | Submit newly scraped price to InfluxDB. |
| `PATCH`| `/api/products/:id/status`| Update scraper task status (`SUCCESS`/`FAILED`/`QUEUED`). |
| `POST` | `/api/logs` | Ingest scraper error logs. |

### AI Analyst Webhooks (Python FastAPI)
| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/webhook/line` | Receive LINE Messaging API Postback events (Feedback voting). |
| `POST` | `/analyze/on-demand` | Manually trigger AI Analysis for a specific product. |

---

## 📜 License
This project is licensed under the MIT License.
