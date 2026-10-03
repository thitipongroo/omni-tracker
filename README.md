# 🚀 Omni-Tracker: Market Intelligence Platform (v2.0)

[![Go](https://img.shields.io/badge/Go-00ADD8?style=flat&logo=go&logoColor=white)](https://go.dev/)
[![Node.js](https://img.shields.io/badge/Node.js-339933?style=flat&logo=node.js&logoColor=white)](https://nodejs.org/)
[![Playwright](https://img.shields.io/badge/Playwright-2EAD33?style=flat&logo=playwright&logoColor=white)](https://playwright.dev/)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-316192?style=flat&logo=postgresql&logoColor=white)](https://www.postgresql.org/)
[![Redis](https://img.shields.io/badge/Redis-DC382D?style=flat&logo=redis&logoColor=white)](https://redis.io/)
[![InfluxDB](https://img.shields.io/badge/InfluxDB-22ADF6?style=flat&logo=influxdb&logoColor=white)](https://www.influxdata.com/)
[![Docker](https://img.shields.io/badge/Docker-2496ED?style=flat&logo=docker&logoColor=white)](https://www.docker.com/)
[![GitHub Actions](https://img.shields.io/badge/GitHub_Actions-2088FF?style=flat&logo=github-actions&logoColor=white)](https://github.com/features/actions)

**Omni-Tracker** is a distributed, high-concurrency market intelligence platform. It treats web scrapers as "IoT sensors" that ingest thousands of data points concurrently. Version 2.0 introduces a robust microservices architecture featuring a centralized Database, Caching layer, and a beautiful Web Dashboard.

## ✨ Features (v2.0)

- **High-Concurrency Ingestion:** Built with Go Fiber to handle massive traffic spikes.
- **Relational Task Management:** Uses **PostgreSQL + GORM** to dynamically manage and distribute scraping tasks via REST API.
- **Ultra-Fast Alerting Engine:** Utilizes **Redis** as an in-memory cache to evaluate price drops in milliseconds, drastically reducing read-load on the primary time-series DB.
- **Time-Series Storage:** Stores historical price trends and fluctuations in **InfluxDB** for deep analytics.
- **Advanced Headless Scraping:** Leverages **Playwright + Stealth Plugin** to bypass modern e-commerce anti-bot protections.
- **Smart Concurrency & Looping:** Node.js worker automatically loops via `node-cron` and processes tasks in concurrent chunks using `Promise.all`.
- **Glassmorphism Dashboard:** A stunning Vanilla HTML/CSS/JS frontend to manage tracked products and view active scraping tasks.
- **Automated CI/CD:** GitHub Actions pipeline for linting, testing, and building Docker images.
- **Event-Driven Alerts:** Real-time notifications via LINE Messaging API when prices drop.

---

## 🏗️ System Architecture

```mermaid
flowchart LR
    subgraph Automation [CI/CD]
        CI[GitHub Actions]
    end

    subgraph Frontend [UI Layer]
        DASH[Web Dashboard]
    end

    subgraph Scrapers [Scraper Fleet - Node.js]
        S1[Playwright Worker]
    end

    subgraph Core [Core API - Go]
        GO["API Gateway\n& Alert Engine"]
    end
    
    subgraph Data [Data Layer]
        PG[(PostgreSQL\nTasks/Config)]
        REDIS[(Redis\nPrice Cache)]
        INFLUX[(InfluxDB\nTime-Series)]
    end
    
    subgraph External [External Services]
        LINE[LINE Notify]
        ECOM1[Shopee / Lazada]
    end

    DASH <-->|REST API| GO
    S1 -->|Fetch Tasks| GO
    GO <-->|Query/Update| PG
    S1 -->|Scrape Data| ECOM1
    S1 -->|POST /prices| GO
    GO <-->|Check Last Price| REDIS
    GO -->|Async Write| INFLUX
    GO -->|Webhook| LINE
    CI -.->|Build & Deploy| GO
    CI -.->|Build & Deploy| S1
```

---

## 🚀 Getting Started

### Prerequisites
- Docker & Docker Compose
- LINE Notify Token (Optional, for alerts)

### Installation & Run

1. **Clone the repository:**
   ```bash
   git clone https://github.com/thitipongroo/omni-tracker.git
   cd omni-tracker
   ```

2. **Configure Environment Variables:**
   Edit the `.env` file and insert your `LINE_NOTIFY_TOKEN` (or leave it blank to disable alerts).

3. **Start the ecosystem via Docker:**
   ```bash
   docker-compose up -d --build
   ```
   *This command spins up PostgreSQL, Redis, InfluxDB, the Golang API, and the Scraper Worker.*

4. **Access the Dashboard:**
   Open your browser and navigate to: [http://localhost:3000](http://localhost:3000)

---

## 📁 Project Structure

```text
omni-tracker/
├── .github/workflows/
│   └── ci.yml               # GitHub Actions CI/CD Pipeline
├── api/                     # Golang Backend (Fiber, GORM, Redis, InfluxDB)
│   ├── Dockerfile
│   ├── go.mod
│   └── main.go
├── dashboard/               # Frontend UI (Vanilla HTML/CSS/JS)
│   ├── index.html
│   ├── style.css
│   └── app.js
├── scraper/                 # Node.js Scraper (Playwright, Stealth, Cron)
│   ├── Dockerfile
│   ├── index.js
│   └── package.json
├── docker-compose.yml       # Orchestrates the entire microservice stack
├── .env                     # Configuration and secrets
└── .gitignore               # Ignored files
```

---

## 🤝 Contributing
Contributions are welcome! Please feel free to submit a Pull Request or open an Issue.

## 📝 License
This project is licensed under the MIT License.
