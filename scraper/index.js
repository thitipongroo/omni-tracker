import 'dotenv/config';
import { chromium } from 'playwright-extra';
import stealthPlugin from 'puppeteer-extra-plugin-stealth';
import axios from 'axios';
import Redis from 'ioredis';

const stealth = stealthPlugin();
chromium.use(stealth);

const API_URL = process.env.GO_API_URL || 'http://go-api:3000';
const API_KEY = process.env.API_KEY || 'my-internal-secret-key';
let redisUrl = process.env.REDIS_URL || 'redis:6379';
if (!redisUrl.startsWith('redis://')) redisUrl = 'redis://' + redisUrl;
const CONCURRENCY_LIMIT = 3;

const PROXY_URLS = (process.env.PROXY_URL || '').split(',').map(p => p.trim()).filter(p => p);

const USER_AGENTS = [
    'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36',
    'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119.0.0.0 Safari/537.36',
    'Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:109.0) Gecko/20100101 Firefox/121.0'
];

async function sendLog(taskId, productId, store, message, level = 'INFO') {
    try {
        await axios.post(`${API_URL}/api/internal/logs`, {
            product_pk: taskId,
            product_id: productId,
            store: store,
            message: message,
            level: level
        }, {
            headers: { 'Authorization': `Bearer ${API_KEY}` }
        });
    } catch (error) {
        console.error(`Failed to send log:`, error.message);
    }
}

function createRedisClient() {
    return new Redis(redisUrl);
}

async function reportStatus(taskId, status) {
    try {
        await axios.patch(`${API_URL}/api/internal/products/${taskId}/status`, { status }, {
            headers: { 'Authorization': `Bearer ${API_KEY}` }
        });
    } catch (error) {
        console.error(`❌ Failed to update status for task ${taskId}:`, error.message);
    }
}

async function simulateHumanBehavior(page) {
    const width = 1920, height = 1080;
    await page.mouse.move(Math.random() * width, Math.random() * height, { steps: 10 }).catch(()=>{});
    await page.evaluate(() => window.scrollBy({ top: 500 + Math.random() * 500, behavior: 'smooth' })).catch(()=>{});
    await page.waitForTimeout(1000 + Math.random() * 1000).catch(()=>{});
    await page.evaluate(() => window.scrollBy({ top: -300 - Math.random() * 300, behavior: 'smooth' })).catch(()=>{});
}

async function processTask(browser, task) {
    const randomUA = USER_AGENTS[Math.floor(Math.random() * USER_AGENTS.length)];
    
    // Removed ignoreHTTPSErrors: true as per audit S9
    const contextOptions = {
        userAgent: randomUA,
        viewport: { width: 1920, height: 1080 }
    };
    
    if (PROXY_URLS.length > 0) {
        const randomProxy = PROXY_URLS[Math.floor(Math.random() * PROXY_URLS.length)];
        const proxyConfig = { server: randomProxy };
        try {
            const urlObj = new URL(randomProxy);
            if (urlObj.username || urlObj.password) {
                proxyConfig.server = `${urlObj.protocol}//${urlObj.host}`;
                proxyConfig.username = decodeURIComponent(urlObj.username);
                proxyConfig.password = decodeURIComponent(urlObj.password);
            }
        } catch {}
        contextOptions.proxy = proxyConfig;
        console.log(`🛡️ Assigned Rotating Proxy: ${proxyConfig.server}`);
        await sendLog(task.id, task.product_id, task.store, `Using proxy: ${proxyConfig.server}`, 'INFO');
    }

    const context = await browser.newContext(contextOptions);
    const page = await context.newPage();
    
    // Hard timeout implementation (B5)
    let timeoutId = setTimeout(() => {
        console.error(`Task ${task.id} timed out. Closing context.`);
        context.close().catch(()=>{});
    }, 60000);

    // Abort useless resources (P3)
    await page.route('**/*', r => {
        const type = r.request().resourceType();
        if (['image', 'font', 'media'].includes(type)) {
            r.abort().catch(()=>{});
        } else {
            r.continue().catch(()=>{});
        }
    });

    await page.addInitScript(() => {
        Object.defineProperty(navigator, 'webdriver', { get: () => undefined });
        window.chrome = { runtime: {} };
    });

    try {
        console.log(`🔍 [${task.product_id}] Scraping at ${task.store}...`);
        await sendLog(task.id, task.product_id, task.store, `Started scraping URL: ${task.url}`, 'INFO');
        
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
                } catch {}
            }
            if (url.includes('lazada') && url.includes('/pdp/data') && response.status() === 200) {
                try {
                    const json = await response.json();
                    if (json?.module?.price?.salePrice?.value) {
                        interceptedPrice = parseFloat(json.module.price.salePrice.value);
                    }
                } catch {}
            }
        });

        await page.goto(task.url, { waitUntil: 'domcontentloaded', timeout: 30000 });
        
        await simulateHumanBehavior(page);
        
        // Performance P2: Wait for price interception to happen, or timeout fallback to DOM
        if (interceptedPrice === null) {
            await Promise.race([
                new Promise(resolve => {
                    const checkInterval = setInterval(() => {
                        if (interceptedPrice !== null) {
                            clearInterval(checkInterval);
                            resolve();
                        }
                    }, 500);
                }),
                page.waitForLoadState('networkidle', { timeout: 10000 }).catch(() => {})
            ]);
        }

        let priceValue = 0;
        
        if (interceptedPrice !== null) {
            priceValue = interceptedPrice;
            console.log(`📡 [${task.product_id}] XHR Intercept Success! Price: ${priceValue} THB`);
            await sendLog(task.id, task.product_id, task.store, `Intercepted JSON XHR Price: ${priceValue}`, 'SUCCESS');
        } else if (task.selector) {
            try {
                const locator = page.locator(task.selector).first();
                await locator.waitFor({ state: 'visible', timeout: 5000 });
                const priceText = await locator.innerText();
                priceValue = parseFloat(priceText.replace(/[^0-9.-]+/g, ""));
                if (isNaN(priceValue)) priceValue = 0;
                console.log(`📦 [${task.product_id}] DOM Parse Success! Price: ${priceValue} THB`);
                await sendLog(task.id, task.product_id, task.store, `DOM Parse Fallback Success: ${priceValue}`, 'INFO');
            } catch (err) {
                console.log(`⚠️ API Intercept & Selector '${task.selector}' failed.`);
                await sendLog(task.id, task.product_id, task.store, `DOM locator failed: ${err.message}`, 'WARN');
                throw new Error('Failed to extract price via XHR and DOM', { cause: err });
            }
        } else {
            throw new Error('No price found and no DOM selector provided');
        }

        await axios.post(`${API_URL}/api/internal/prices`, {
            id: task.id,
            price: priceValue
        }, {
            headers: { 'Authorization': `Bearer ${API_KEY}` }
        });
        
        await reportStatus(task.id, 'SUCCESS');
    } catch (error) {
        console.error(`❌ [${task.product_id}] Scraping failed:`, error.message);
        await sendLog(task.id, task.product_id, task.store, `Playwright Error: ${error.message}`, 'ERROR');
        await reportStatus(task.id, 'FAILED');
    } finally {
        clearTimeout(timeoutId);
        await context.close().catch(()=>{});
    }
}

let taskCount = 0;
const MAX_TASKS_BEFORE_RESTART = 500;
let activeTasks = 0;
let shouldRestart = false;

async function runScraperFleet() {
    console.log('🚀 Initializing Evasive Scraper Fleet with Proxy Rotation & API Intercept...');
    let browser = await chromium.launch({ headless: true });
    
    // B6: Handle browser crash
    browser.on('disconnected', () => {
        console.log("Browser disconnected/crashed! Marking for restart.");
        shouldRestart = true;
    });
    
    const redis = createRedisClient();
    console.log(`👷 Main Worker started, processing up to ${CONCURRENCY_LIMIT} tasks concurrently...`);
    
    while (true) {
        if (shouldRestart && activeTasks === 0) {
            console.log('♻️ Restarting browser to safely clear memory...');
            try { await browser.close(); } catch(e) {}
            browser = await chromium.launch({ headless: true });
            browser.on('disconnected', () => {
                console.log("Browser disconnected/crashed! Marking for restart.");
                shouldRestart = true;
            });
            shouldRestart = false;
            taskCount = 0;
        }

        if (activeTasks >= CONCURRENCY_LIMIT || shouldRestart) {
            await new Promise(r => setTimeout(r, 500));
            continue;
        }

        try {
            // Use brpoplpush or blmove if reliable queue is implemented, but here we just pop
            const result = await redis.brpop('scraper_tasks', 2);
            if (result) {
                const task = JSON.parse(Array.isArray(result) ? result[1] : result);
                activeTasks++;
                taskCount++;
                
                if (taskCount >= MAX_TASKS_BEFORE_RESTART) {
                    shouldRestart = true;
                }

                processTask(browser, task)
                    .catch(e => console.error(`👷 Task Error:`, e.message))
                    .finally(() => {
                        activeTasks--;
                    });
            }
        } catch (error) {
            console.error(`👷 Worker encountered error:`, error.message);
            await new Promise(r => setTimeout(r, 5000));
        }
    }
}

runScraperFleet();