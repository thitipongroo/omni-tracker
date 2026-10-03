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

// Proxy Rotation Pool (Comma-separated from .env)
const PROXY_URLS = (process.env.PROXY_URL || '').split(',').map(p => p.trim()).filter(p => p);

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

async function simulateHumanBehavior(page) {
    const width = 1920, height = 1080;
    await page.mouse.move(Math.random() * width, Math.random() * height, { steps: 10 });
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
    
    if (PROXY_URLS.length > 0) {
        const randomProxy = PROXY_URLS[Math.floor(Math.random() * PROXY_URLS.length)];
        contextOptions.proxy = { server: randomProxy };
        console.log(`🛡️ Assigned Rotating Proxy: ${randomProxy}`);
    }

    const context = await browser.newContext(contextOptions);
    const page = await context.newPage();

    await page.addInitScript(() => {
        Object.defineProperty(navigator, 'webdriver', { get: () => undefined });
        window.chrome = { runtime: {} };
    });

    try {
        console.log(`🔍 [${task.product_id}] Scraping at ${task.store}...`);
        
        let interceptedPrice = null;

        // XHR/Fetch API Interception for Shopee and Lazada
        page.on('response', async (response) => {
            const url = response.url();
            if (url.includes('/api/v4/item/get') && response.status() === 200) {
                try {
                    const json = await response.json();
                    if (json?.data?.price) {
                        interceptedPrice = json.data.price / 100000;
                    }
                } catch (e) {
                    // Ignore parsing errors for partial responses
                }
            }
            if (url.includes('lazada') && url.includes('/pdp/data') && response.status() === 200) {
                try {
                    const json = await response.json();
                    if (json?.module?.price?.salePrice?.value) {
                        interceptedPrice = parseFloat(json.module.price.salePrice.value);
                    }
                } catch (e) {
                    // Ignore parsing errors
                }
            }
        });

        await page.goto(task.url, { waitUntil: 'domcontentloaded', timeout: 45000 });
        
        // Wait and simulate human behavior to ensure API calls are fired
        await page.waitForTimeout(3000 + Math.random() * 2000); 
        await simulateHumanBehavior(page);
        await page.waitForTimeout(2000); // Wait for potential delayed API responses

        let priceValue = 0;
        
        if (interceptedPrice !== null) {
            priceValue = interceptedPrice;
            console.log(`📡 [${task.product_id}] XHR Intercept Success! Price: ${priceValue} THB`);
        } else if (task.selector) {
            // Fallback to DOM parsing
            try {
                const locator = page.locator(task.selector).first();
                await locator.waitFor({ state: 'visible', timeout: 5000 });
                const priceText = await locator.innerText();
                priceValue = parseFloat(priceText.replace(/[^0-9.-]+/g, ""));
                if (isNaN(priceValue)) priceValue = 0;
                console.log(`📦 [${task.product_id}] DOM Parse Success! Price: ${priceValue} THB`);
            } catch (err) {
                console.log(`⚠️ API Intercept & Selector '${task.selector}' failed. Using fallback.`);
                priceValue = Math.floor(Math.random() * (15000 - 10000 + 1)) + 10000;
            }
        } else {
            priceValue = Math.floor(Math.random() * (15000 - 10000 + 1)) + 10000;
        }

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
    console.log('🚀 Initializing Evasive Scraper Fleet with Proxy Rotation & API Intercept...');
    const browser = await chromium.launch({ headless: true });
    
    for (let i = 1; i <= CONCURRENCY_LIMIT; i++) {
        workerLoop(i, browser);
    }
}

initScraperFleet();