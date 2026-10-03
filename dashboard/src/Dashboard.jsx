import React, { useState, useEffect } from 'react';
import axios from 'axios';
import { LogOut, Plus, Trash2, Terminal } from 'lucide-react';

export default function Dashboard({ token, setToken }) {
  const [products, setProducts] = useState([]);
  const [logs, setLogs] = useState([]);
  const [showLogs, setShowLogs] = useState(false);
  const [form, setForm] = useState({ product_id: '', store: 'shopee', url: '' });

  const api = axios.create({
    baseURL: 'http://localhost:3000/api',
    headers: { Authorization: `Bearer ${token}` }
  });

  const fetchData = async () => {
    try {
      const [prodRes, logRes] = await Promise.all([api.get('/products'), api.get('/logs')]);
      setProducts(prodRes.data || []);
      setLogs(logRes.data || []);
    } catch (e) {
      if (e.response?.status === 401) setToken(null);
    }
  };

  useEffect(() => { fetchData(); }, []);

  const addProduct = async (e) => {
    e.preventDefault();
    await api.post('/products', form);
    setForm({ product_id: '', store: 'shopee', url: '' });
    fetchData();
  };

  const deleteProduct = async (id) => {
    if (confirm('Delete this tracker?')) {
      await api.delete(`/products/${id}`);
      fetchData();
    }
  };

  return (
    <div className="p-8 max-w-7xl mx-auto">
      <div className="flex justify-between items-center mb-10">
        <h1 className="text-4xl font-bold bg-clip-text text-transparent bg-gradient-to-r from-primary to-secondary">
          Omni-Tracker Dashboard
        </h1>
        <div className="flex gap-4">
          <button onClick={() => setShowLogs(!showLogs)} className="flex items-center gap-2 bg-slate-800 hover:bg-slate-700 px-4 py-2 rounded-lg transition-colors border border-slate-700">
            <Terminal size={18} /> {showLogs ? 'Hide Logs' : 'View Logs'}
          </button>
          <button onClick={() => setToken(null)} className="flex items-center gap-2 bg-red-500/10 hover:bg-red-500/20 text-red-400 px-4 py-2 rounded-lg transition-colors border border-red-500/20">
            <LogOut size={18} /> Logout
          </button>
        </div>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-8">
        <div className="lg:col-span-1 space-y-6">
          <div className="bg-darker p-6 rounded-2xl border border-slate-800 shadow-xl">
            <h2 className="text-xl font-semibold mb-6 flex items-center gap-2 text-white"><Plus size={20}/> New Tracker</h2>
            <form onSubmit={addProduct} className="space-y-4">
              <input type="text" placeholder="Product ID" value={form.product_id} onChange={e => setForm({...form, product_id: e.target.value})} className="w-full bg-slate-900 border border-slate-700 rounded-lg p-3 text-white focus:ring-2 focus:ring-primary outline-none" required />
              <select value={form.store} onChange={e => setForm({...form, store: e.target.value})} className="w-full bg-slate-900 border border-slate-700 rounded-lg p-3 text-white focus:ring-2 focus:ring-primary outline-none">
                <option value="shopee">Shopee</option>
                <option value="lazada">Lazada</option>
              </select>
              <input type="url" placeholder="Product URL" value={form.url} onChange={e => setForm({...form, url: e.target.value})} className="w-full bg-slate-900 border border-slate-700 rounded-lg p-3 text-white focus:ring-2 focus:ring-primary outline-none" required />
              <button type="submit" className="w-full bg-gradient-to-r from-primary to-secondary text-white font-bold py-3 rounded-lg hover:opacity-90 transition-all shadow-lg shadow-primary/20">Track Product</button>
            </form>
          </div>
        </div>

        <div className="lg:col-span-2">
          {showLogs ? (
            <div className="bg-darker rounded-2xl border border-slate-800 shadow-xl overflow-hidden flex flex-col h-[600px]">
               <div className="p-4 border-b border-slate-800 bg-slate-900 flex items-center gap-2 font-mono text-sm text-slate-400">
                  <Terminal size={16} className="text-primary"/> System Diagnostics
               </div>
               <div className="p-4 flex-1 overflow-y-auto font-mono text-sm space-y-2">
                 {logs.map(log => (
                   <div key={log.id} className="flex gap-4 p-2 rounded hover:bg-slate-800/50 transition-colors">
                     <span className="text-slate-500 whitespace-nowrap">{new Date(log.created_at).toLocaleTimeString()}</span>
                     <span className={`w-16 ${log.level === 'ERROR' ? 'text-red-400' : log.level === 'SUCCESS' ? 'text-green-400' : log.level === 'WARN' ? 'text-yellow-400' : 'text-blue-400'}`}>[{log.level}]</span>
                     <span className="text-slate-300">[{log.store}] {log.product_id}: {log.message}</span>
                   </div>
                 ))}
                 {logs.length === 0 && <div className="text-slate-500 text-center py-10">No logs generated yet. Wait for scraper to run.</div>}
               </div>
            </div>
          ) : (
            <div className="bg-darker rounded-2xl border border-slate-800 shadow-xl overflow-hidden">
              <table className="w-full text-left">
                <thead className="bg-slate-900/50 border-b border-slate-800 text-slate-400">
                  <tr>
                    <th className="p-4 font-medium">Product ID</th>
                    <th className="p-4 font-medium">Store</th>
                    <th className="p-4 font-medium">Status</th>
                    <th className="p-4 font-medium">Last Scraped</th>
                    <th className="p-4 font-medium text-right">Actions</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-slate-800">
                  {products.map(p => (
                    <tr key={p.id} className="hover:bg-slate-800/30 transition-colors">
                      <td className="p-4 font-mono text-primary font-medium">{p.product_id}</td>
                      <td className="p-4 capitalize text-slate-300">{p.store}</td>
                      <td className="p-4">
                        <span className={`px-3 py-1 rounded-full text-xs font-medium border ${p.status === 'SUCCESS' ? 'bg-green-500/10 text-green-400 border-green-500/20' : p.status === 'ANOMALY' ? 'bg-yellow-500/10 text-yellow-400 border-yellow-500/20' : p.status === 'FAILED' ? 'bg-red-500/10 text-red-400 border-red-500/20' : 'bg-slate-800 text-slate-400 border-slate-700'}`}>
                          {p.status}
                        </span>
                      </td>
                      <td className="p-4 text-sm text-slate-400">
                        {p.last_scraped_at ? new Date(p.last_scraped_at).toLocaleString() : 'Pending...'}
                      </td>
                      <td className="p-4 text-right">
                        <button onClick={() => deleteProduct(p.id)} className="text-slate-500 hover:text-red-400 transition-colors p-2 rounded-lg hover:bg-red-500/10">
                          <Trash2 size={18} />
                        </button>
                      </td>
                    </tr>
                  ))}
                  {products.length === 0 && (
                    <tr>
                      <td colSpan="5" className="p-8 text-center text-slate-500">No products tracked yet. Add one from the left panel.</td>
                    </tr>
                  )}
                </tbody>
              </table>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
