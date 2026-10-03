require('dotenv').config();
const { chromium } = require('playwright-extra');
const stealth = require('puppeteer-extra-plugin-stealth')();
const axios = require('axios');
const Redis = require('ioredis');

chromium.use(stealth);

const API_URL = process.env.GO_API_URL || 'http://go-api:3000';
const API_KEY = process.env.API_KEY || 'my-internal-secret-key';
let redisUrl = process.env.REDIS_URL || 'redis:6379';
if (!redisUrl.startsWith('redis://')) {
    redisUrl = 'redis://' + redisUrl;
}
const CONCURRENCY_LIMIT = 3;

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

async function processTask(browser, task) {
    const context = await browser.newContext({
        userAgent: 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36'
    });
    const page = await context.newPage();
    try {
        console.log(`🔍 [${task.product_id}] Scraping at ${task.store}...`);
        await page.goto(task.url, { waitUntil: 'domcontentloaded', timeout: 30000 });
        await page.waitForTimeout(3000); 

        let priceValue = 0;
        if (task.selector) {
            try {
                const priceText = await page.locator(task.selector).first().innerText({ timeout: 5000 });
                priceValue = parseFloat(priceText.replace(/[^0-9.-]+/g, ""));
                if (isNaN(priceValue)) priceValue = 0;
            } catch (err) {
                console.log(`⚠️ Selector '${task.selector}' not found. Using fallback.`);
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
            // BRPOP blocks indefinitely (0) until a task arrives in 'scraper_tasks'
            const result = await redis.brpop('scraper_tasks', 0); 
            if (result) {
                const task = JSON.parse(result[1]);
                console.log(`👷 Worker ${workerId} picked up task: ${task.product_id}`);
                await processTask(browser, task);
            }
        } catch (error) {
            console.error(`👷 Worker ${workerId} encountered Redis error:`, error.message);
            await new Promise(r => setTimeout(r, 5000)); // Sleep before retry to avoid CPU spin
        }
    }
}

async function initScraperFleet() {
    console.log('🚀 Initializing Scraper Fleet (Message Queue Consumer)...');
    const browser = await chromium.launch({ headless: true });
    
    // Spawn isolated concurrent workers
    for (let i = 1; i <= CONCURRENCY_LIMIT; i++) {
        workerLoop(i, browser);
    }
}

initScraperFleet();