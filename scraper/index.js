require('dotenv').config();
const { chromium } = require('playwright-extra');
const stealth = require('puppeteer-extra-plugin-stealth')();
const axios = require('axios');

// Add stealth plugin to evade anti-bot detections
chromium.use(stealth);

const API_URL = process.env.GO_API_URL || 'http://go-api:3000';
const API_KEY = process.env.API_KEY || 'my-internal-secret-key';

async function fetchTasks() {
    try {
        const response = await axios.get(`${API_URL}/api/tasks`, {
            headers: { 'Authorization': `Bearer ${API_KEY}` }
        });
        return response.data;
    } catch (error) {
        console.error('❌ Failed to fetch tasks from Go API:', error.message);
        return [];
    }
}

async function scrapePrice(page, task) {
    try {
        console.log(`🔍 Scraping ${task.product_id} at ${task.store}...`);
        await page.goto(task.url, { waitUntil: 'domcontentloaded', timeout: 30000 });
        
        // Polite delay to let JS execution and network requests settle
        await page.waitForTimeout(3000); 

        let priceValue = 0;
        
        // -------------------------------------------------------------------
        // TODO: In a real scenario, use specific selectors per store.
        // Example: 
        // if (task.store === 'shopee') { ... }
        // const priceText = await page.locator('.product-price').first().innerText();
        // priceValue = parseFloat(priceText.replace(/[^0-9.-]+/g, ""));
        // -------------------------------------------------------------------

        // For demonstration, we simulate finding a dynamic price
        // Randomize price slightly to simulate price drops and trigger alerts
        priceValue = Math.floor(Math.random() * (15000 - 10000 + 1)) + 10000;
        
        console.log(`📦 Found Price for ${task.product_id}: ${priceValue} THB`);
        return priceValue;

    } catch (error) {
        console.error(`❌ Scraping failed for ${task.product_id}:`, error.message);
        return null;
    }
}

async function submitPrice(task, price) {
    try {
        const payload = {
            product_id: task.product_id,
            store: task.store,
            price: price,
            url: task.url
        };
        
        await axios.post(`${API_URL}/api/prices`, payload, {
            headers: { 'Authorization': `Bearer ${API_KEY}` }
        });
        console.log(`✅ Sent to Ingestion API: ${task.product_id}`);
    } catch (error) {
        console.error(`❌ Failed to submit price for ${task.product_id}:`, error.message);
    }
}

(async () => {
    console.log('🚀 Starting E-commerce Scraper with Stealth Mode...');
    
    // 1. Fetch dynamic tasks from the Queue/API
    const tasks = await fetchTasks();
    if (!tasks || tasks.length === 0) {
        console.log('💤 No tasks to process. Exiting.');
        return;
    }

    // 2. Launch Stealth Browser
    const browser = await chromium.launch({ 
        headless: true,
        // Optional: Add proxy support here
        // args: ['--proxy-server=http://your-proxy-here:port'] 
    });
    
    // Set a realistic User-Agent
    const context = await browser.newContext({
        userAgent: 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36'
    });
    
    const page = await context.newPage();

    // 3. Iterate and process tasks
    for (const task of tasks) {
        const price = await scrapePrice(page, task);
        if (price !== null) {
            await submitPrice(task, price);
        }
        
        // Polite delay between requests to avoid rate-limiting
        await page.waitForTimeout(2000);
    }

    await browser.close();
    console.log('🏁 Scraping finished.');
})();