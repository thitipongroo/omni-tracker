# 🚀 Omni-Tracker: High-Speed Price & Market Intelligence Platform

[![Go](https://img.shields.io/badge/Go-00ADD8?style=flat&logo=go&logoColor=white)](https://go.dev/)
[![Playwright](https://img.shields.io/badge/Playwright-2EAD33?style=flat&logo=playwright&logoColor=white)](https://playwright.dev/)
[![InfluxDB](https://img.shields.io/badge/InfluxDB-22ADF6?style=flat&logo=influxdb&logoColor=white)](https://www.influxdata.com/)
[![Docker](https://img.shields.io/badge/Docker-2496ED?style=flat&logo=docker&logoColor=white)](https://www.docker.com/)
[![GitHub Actions](https://img.shields.io/badge/GitHub_Actions-2088FF?style=flat&logo=github-actions&logoColor=white)](https://github.com/features/actions)

**Omni-Tracker** is a distributed, high-concurrency market intelligence platform. It treats web scrapers as "IoT sensors" that ingest thousands of data points (prices, stock statuses) concurrently into a Golang-based API, storing the metrics in a time-series database for trend analysis and real-time alerts.

## ✨ Features

- **High-Concurrency Ingestion:** Built with Go Fiber to handle massive traffic spikes from parallel scraping workers without bottlenecking.
- **Time-Series Storage:** Utilizes InfluxDB for optimized storage and querying of historical price changes and market trends.
- **Headless Browser Automation:** Leverages Playwright to bypass modern e-commerce anti-bot protections and extract dynamic data.
- **Event-Driven Alerts:** Evaluates price drops in real-time and triggers notifications via the LINE Messaging API.
- **Fully Containerized:** The entire ecosystem (Go API, Node.js Scrapers, InfluxDB) is orchestrated via Docker Compose.
- **Automated CI/CD:** GitHub Actions pipeline for linting, testing, and building Docker images on every push.

---

## 🏗️ System Architecture

```mermaid
flowchart LR
    subgraph CICD [CI/CD]
        CI[GitHub Actions]
    end

    subgraph Scrapers [Scraper Fleet - Node.js and Playwright]
        S1[Scraper Worker 1]
        S2[Scraper Worker 2]
    end

    subgraph Core [Core Platform - Golang and InfluxDB]
        GO["Go Fiber API (High Concurrency)"]
        DB[("InfluxDB (Time-Series)")]
        ALERT[Alert Engine]
    end
    
    subgraph External [External Services]
        LINE[LINE Bot API]
        ECOM[Shopee / Lazada]
    end

    S1 -->|Scrape| ECOM
    S1 -->|POST /api/prices| GO
    S2 -->|POST /api/prices| GO
    GO -->|Batch Write| DB
    ALERT -->|Query Trends| DB
    ALERT -->|Trigger Webhook| LINE
    CI -.->|Automated Build & Test| GO
```

---

## 🚀 Getting Started

### Prerequisites
- Docker & Docker Compose
- Go 1.21+
- Node.js 18+

### Installation & Run

1. **Clone the repository:**
   ```bash
   git clone https://github.com/thitipongroo/omni-tracker.git
   cd omni-tracker
   ```

2. **Start the ecosystem via Docker:**
   ```bash
   docker-compose up -d --build
   ```
   *This command spins up the InfluxDB instance, the Golang API on port 3000, and the Playwright scraper workers.*

3. **Verify the API is running:**
   ```bash
   curl http://localhost:3000/health
   ```

---

## 📁 Project Structure

- `/api` - Golang ingestion API and alerting engine.
- `/scraper` - Node.js and Playwright scraping scripts.
- `/.github/workflows` - CI/CD pipelines.
- `docker-compose.yml` - Infrastructure orchestration.

---

## 🤝 Contributing
Contributions are welcome! Please feel free to submit a Pull Request or open an Issue.

## 📝 License
This project is licensed under the MIT License.
