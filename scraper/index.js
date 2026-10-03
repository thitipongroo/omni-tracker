const { chromium } = require('playwright');
const axios = require('axios');

(async () => {
    console.log('🚀 Starting E-commerce Scraper...');
    const browser = await chromium.launch({ headless: true });
    const page = await browser.newPage();

    try {
        // Example: Go to a dummy product page (Replace with real URL)
        await page.goto('https://example-ecommerce.com/product/123');

        // Wait for price element and extract text
        // Note: Real Shopee/Lazada selectors change often
        const priceText = await page.locator('.product-price').innerText();
        const priceValue = parseFloat(priceText.replace(/[^0-9.-]+/g, ""));

        const payload = {
            product_id: "iphone-15-pro",
            store: "shopee",
            price: priceValue
        };

        console.log(`📦 Scraped Price: ${payload.price} THB`);

        // Send to Go High-Speed API
        await axios.post('http://go-api:3000/api/prices', payload);
        console.log('✅ Sent to Ingestion API');

    } catch (error) {
        console.error('❌ Scraping failed:', error.message);
    } finally {
        await browser.close();
    }
})();