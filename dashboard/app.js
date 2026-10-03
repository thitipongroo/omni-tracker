const API_BASE = '/api';
let TOKEN = localStorage.getItem('omnitracker_token');

const loginOverlay = document.getElementById('loginOverlay');
const mainContent = document.getElementById('mainContent');
const chartOverlay = document.getElementById('chartOverlay');
let chartInstance = null;

if (TOKEN) {
    loginOverlay.classList.remove('active');
    mainContent.style.display = 'block';
    fetchProducts();
}

document.getElementById('loginForm').addEventListener('submit', async (e) => {
    e.preventDefault();
    const pw = document.getElementById('password').value;
    const res = await fetch(`${API_BASE}/login`, {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({password: pw})
    });
    if (res.ok) {
        const data = await res.json();
        TOKEN = data.token;
        localStorage.setItem('omnitracker_token', TOKEN);
        loginOverlay.classList.remove('active');
        mainContent.style.display = 'block';
        fetchProducts();
    } else {
        alert('Invalid Password');
    }
});

function logout() {
    localStorage.removeItem('omnitracker_token');
    location.reload();
}

const authHeaders = () => ({
    'Content-Type': 'application/json',
    'Authorization': `Bearer ${TOKEN}`
});

async function fetchProducts() {
    try {
        const res = await fetch(`${API_BASE}/products`, { headers: authHeaders() });
        if (res.status === 401) return logout();
        const products = await res.json();
        const tbody = document.getElementById('productsList');
        tbody.innerHTML = '';
        
        products.forEach(p => {
            const time = p.last_scraped_at ? new Date(p.last_scraped_at).toLocaleString() : 'Never';
            const tr = document.createElement('tr');
            tr.innerHTML = `
                <td>#${p.id}</td>
                <td><strong>${p.product_id}</strong></td>
                <td>${p.store.toUpperCase()}</td>
                <td class="status-${p.status}"><strong>${p.status}</strong></td>
                <td><small>${time}</small></td>
                <td>
                    <button class="btn-chart" onclick="viewChart('${p.product_id}')">📈 Chart</button>
                    <button class="btn-delete" onclick="deleteProduct(${p.id})">Remove</button>
                </td>
            `;
            tbody.appendChild(tr);
        });
    } catch (error) {
        console.error("Failed to load products", error);
    }
}

async function addProduct(e) {
    e.preventDefault();
    await fetch(`${API_BASE}/products`, {
        method: 'POST',
        headers: authHeaders(),
        body: JSON.stringify({
            product_id: document.getElementById('productId').value,
            store: document.getElementById('store').value,
            url: document.getElementById('productUrl').value
        })
    });
    document.getElementById('addProductForm').reset();
    fetchProducts();
}

async function deleteProduct(id) {
    if (confirm('Delete this task?')) {
        await fetch(`${API_BASE}/products/${id}`, { method: 'DELETE', headers: authHeaders() });
        fetchProducts();
    }
}

document.getElementById('configForm').addEventListener('submit', async (e) => {
    e.preventDefault();
    await fetch(`${API_BASE}/configs`, {
        method: 'POST',
        headers: authHeaders(),
        body: JSON.stringify({
            store: document.getElementById('configStore').value,
            selector: document.getElementById('configSelector').value
        })
    });
    alert('Config updated successfully!');
    document.getElementById('configForm').reset();
});

async function viewChart(productId) {
    chartOverlay.classList.add('active');
    document.getElementById('chartTitle').innerText = `Price History: ${productId}`;
    
    const res = await fetch(`${API_BASE}/history/${productId}`, { headers: authHeaders() });
    const data = await res.json();
    
    const labels = data ? data.map(d => new Date(d.time).toLocaleDateString()) : [];
    const prices = data ? data.map(d => d.price) : [];

    if (chartInstance) chartInstance.destroy();
    const ctx = document.getElementById('priceChart').getContext('2d');
    chartInstance = new Chart(ctx, {
        type: 'line',
        data: {
            labels,
            datasets: [{
                label: 'Price (THB)',
                data: prices,
                borderColor: '#3b82f6',
                tension: 0.1,
                fill: true,
                backgroundColor: 'rgba(59, 130, 246, 0.1)'
            }]
        },
        options: {
            responsive: true,
            scales: {
                y: { beginAtZero: false }
            }
        }
    });
}

document.getElementById('closeChart').addEventListener('click', () => {
    chartOverlay.classList.remove('active');
});

document.getElementById('addProductForm').addEventListener('submit', addProduct);
