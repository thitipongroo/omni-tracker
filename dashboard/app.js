const API_BASE = '/api';

async function fetchProducts() {
    try {
        const res = await fetch(`${API_BASE}/products`);
        const products = await res.json();
        const tbody = document.getElementById('productsList');
        tbody.innerHTML = '';
        
        products.forEach(p => {
            const tr = document.createElement('tr');
            tr.innerHTML = `
                <td>#${p.id}</td>
                <td><strong>${p.product_id}</strong></td>
                <td>${p.store.toUpperCase()}</td>
                <td><span class="badge">Active</span></td>
                <td><button class="btn-delete" onclick="deleteProduct(${p.id})">Remove</button></td>
            `;
            tbody.appendChild(tr);
        });
    } catch (error) {
        console.error("Failed to load products", error);
    }
}

async function addProduct(e) {
    e.preventDefault();
    const btn = e.target.querySelector('button');
    btn.innerText = 'Adding...';

    const payload = {
        product_id: document.getElementById('productId').value,
        store: document.getElementById('store').value,
        url: document.getElementById('productUrl').value
    };

    try {
        await fetch(`${API_BASE}/products`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(payload)
        });
        document.getElementById('addProductForm').reset();
        await fetchProducts();
    } catch (error) {
        alert("Failed to add product");
    } finally {
        btn.innerText = 'Add Task';
    }
}

async function deleteProduct(id) {
    if (confirm('Are you sure you want to delete this scrape task?')) {
        try {
            await fetch(`${API_BASE}/products/${id}`, { method: 'DELETE' });
            await fetchProducts();
        } catch (error) {
            alert("Failed to delete product");
        }
    }
}

document.getElementById('addProductForm').addEventListener('submit', addProduct);

// Initial Load
fetchProducts();
