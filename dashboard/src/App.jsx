import React, { useState, useEffect } from 'react';
import Login from './Login';
import Dashboard from './Dashboard';

function App() {
  const [token, setToken] = useState(localStorage.getItem('omnitracker_token'));

  useEffect(() => {
    if (token) {
      localStorage.setItem('omnitracker_token', token);
    } else {
      localStorage.removeItem('omnitracker_token');
    }
  }, [token]);

  return (
    <div className="min-h-screen bg-dark text-slate-200 font-sans">
      {token ? <Dashboard token={token} setToken={setToken} /> : <Login setToken={setToken} />}
    </div>
  );
}

export default App;
