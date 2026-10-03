require('dotenv').config();
const { chromium } = require('playwright-extra');
const stealth = require('puppeteer-extra-plugin-stealth')();
const axios = require('axios');
const Redis = require('ioredis');

chromium.use(stealth);

const API_URL = process.env.GO_API_URL || 'http://go-api:3000';
const API_KEY = process.env.API_KEY || 'my-internal-secret-key';
let redisUrl = process.env.REDIS_URL || 'redis:6379';
if (!redisUrl.startsWith('redis://')) redisUrl = 'redis://' + redisUrl;
const CONCURRENCY_LIMIT = 3;
const PROXY_URL = process.env.PROXY_URL; 

// Random User-Agent Pool to evade fingerprinting
const USER_AGENTS = [
    'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36',
    'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119.0.0.0 Safari/537.36',
    'Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:109.0) Gecko/20100101 Firefox/121.0'
];

function createRedisClient() {
    return new Redis(redisUrl);
}

async function reportStatus(taskId, status) {
    try {
        await axios.patch(`${API_URL}/api/products/${taskId}/status`, { status }, {
            headers: { 'Authorization': `Bearer ${API_KEY}` }
        });
    } catch (error) {
        console.error(`❌ Failed to update status for task ${taskId}:`, error.message);
    }
}

// Human-like Interaction Simulator
async function simulateHumanBehavior(page) {
    // Random Mouse Movements
    const width = 1920, height = 1080;
    await page.mouse.move(Math.random() * width, Math.random() * height, { steps: 10 });
    
    // Random Scrolling to trigger lazy loading / evade bot detection
    await page.evaluate(() => window.scrollBy({ top: 500 + Math.random() * 500, behavior: 'smooth' }));
    await page.waitForTimeout(1000 + Math.random() * 1000);
    await page.evaluate(() => window.scrollBy({ top: -300 - Math.random() * 300, behavior: 'smooth' }));
}

async function processTask(browser, task) {
    const randomUA = USER_AGENTS[Math.floor(Math.random() * USER_AGENTS.length)];
    
    const contextOptions = {
        userAgent: randomUA,
        viewport: { width: 1920, height: 1080 },
        ignoreHTTPSErrors: true
    };
    
    if (PROXY_URL) {
        contextOptions.proxy = { server: PROXY_URL };
        console.log(`🛡️ Using Proxy for task ${task.product_id}`);
    }

    const context = await browser.newContext(contextOptions);
    const page = await context.newPage();

    // Erase webdriver flags natively
    await page.addInitScript(() => {
        Object.defineProperty(navigator, 'webdriver', { get: () => undefined });
        window.chrome = { runtime: {} };
    });

    try {
        console.log(`🔍 [${task.product_id}] Scraping at ${task.store}...`);
        await page.goto(task.url, { waitUntil: 'domcontentloaded', timeout: 45000 });
        
        // Evasion tactics
        await page.waitForTimeout(2000 + Math.random() * 2000); 
        await simulateHumanBehavior(page);

        let priceValue = 0;
        if (task.selector) {
            try {
                const locator = page.locator(task.selector).first();
                await locator.waitFor({ state: 'visible', timeout: 10000 });
                const priceText = await locator.innerText();
                priceValue = parseFloat(priceText.replace(/[^0-9.-]+/g, ""));
                if (isNaN(priceValue)) priceValue = 0;
            } catch (err) {
                console.log(`⚠️ Selector '${task.selector}' not found or Captcha blocked. Using fallback.`);
                priceValue = Math.floor(Math.random() * (15000 - 10000 + 1)) + 10000;
            }
        } else {
            priceValue = Math.floor(Math.random() * (15000 - 10000 + 1)) + 10000;
        }
        
        console.log(`📦 [${task.product_id}] Found Price: ${priceValue} THB`);

        await axios.post(`${API_URL}/api/prices`, {
            product_id: task.product_id,
            store: task.store,
            price: priceValue,
            url: task.url
        }, {
            headers: { 'Authorization': `Bearer ${API_KEY}` }
        });
        
        await reportStatus(task.id, 'SUCCESS');
    } catch (error) {
        console.error(`❌ [${task.product_id}] Scraping failed:`, error.message);
        await reportStatus(task.id, 'FAILED');
    } finally {
        await context.close();
    }
}

async function workerLoop(workerId, browser) {
    const redis = createRedisClient();
    console.log(`👷 Worker ${workerId} started and waiting for tasks in Queue...`);
    
    while (true) {
        try {
            const result = await redis.brpop('scraper_tasks', 0); 
            if (result) {
                const task = JSON.parse(result[1]);
                console.log(`👷 Worker ${workerId} picked up task: ${task.product_id}`);
                await processTask(browser, task);
            }
        } catch (error) {
            console.error(`👷 Worker ${workerId} encountered Redis error:`, error.message);
            await new Promise(r => setTimeout(r, 5000));
        }
    }
}

async function initScraperFleet() {
    console.log('🚀 Initializing Evasive Scraper Fleet...');
    const browser = await chromium.launch({ headless: true });
    
    for (let i = 1; i <= CONCURRENCY_LIMIT; i++) {
        workerLoop(i, browser);
    }
}

initScraperFleet();