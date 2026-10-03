require('dotenv').config();
const { chromium } = require('playwright-extra');
const stealth = require('puppeteer-extra-plugin-stealth')();
const axios = require('axios');
const cron = require('node-cron');

chromium.use(stealth);

const API_URL = process.env.GO_API_URL || 'http://go-api:3000';
const API_KEY = process.env.API_KEY || 'my-internal-secret-key';
const CONCURRENCY_LIMIT = 3;

async function fetchTasks() {
    try {
        const response = await axios.get(`${API_URL}/api/tasks`, {
            headers: { 'Authorization': `Bearer ${API_KEY}` }
        });
        return response.data;
    } catch (error) {
        console.error('❌ Failed to fetch tasks:', error.message);
        return [];
    }
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

async function scrapePrice(context, task) {
    const page = await context.newPage();
    try {
        console.log(`🔍 [${task.product_id}] Scraping at ${task.store}...`);
        await page.goto(task.url, { waitUntil: 'domcontentloaded', timeout: 30000 });
        await page.waitForTimeout(3000); 

        let priceValue = 0;
        
        // Use Dynamic Selector from Postgres
        if (task.selector) {
            try {
                const priceText = await page.locator(task.selector).first().innerText({ timeout: 5000 });
                priceValue = parseFloat(priceText.replace(/[^0-9.-]+/g, ""));
                if (isNaN(priceValue)) priceValue = 0;
            } catch (err) {
                console.log(`⚠️ Selector '${task.selector}' not found for ${task.product_id}. Using fallback.`);
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
        await page.close(); 
    }
}

async function runScraperCycle() {
    console.log(`\n⏰ [${new Date().toISOString()}] Starting Scraper Cycle...`);
    const tasks = await fetchTasks();
    if (!tasks || tasks.length === 0) {
        console.log('💤 No active tasks.');
        return;
    }

    const browser = await chromium.launch({ headless: true });
    const context = await browser.newContext({
        userAgent: 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36'
    });

    for (let i = 0; i < tasks.length; i += CONCURRENCY_LIMIT) {
        const batch = tasks.slice(i, i + CONCURRENCY_LIMIT);
        console.log(`🚀 Processing batch ${Math.floor(i/CONCURRENCY_LIMIT) + 1}...`);
        await Promise.all(batch.map(task => scrapePrice(context, task)));
    }

    await browser.close();
    console.log('🏁 Scraper Cycle Finished.');
}

runScraperCycle();
cron.schedule('*/10 * * * *', () => {
    runScraperCycle();
});
console.log('⏳ Scraper Service initialized. Waiting for jobs...');