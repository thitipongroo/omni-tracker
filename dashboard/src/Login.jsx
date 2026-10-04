import React, { useState } from 'react';
import axios from 'axios';
import { KeyRound, UserPlus } from 'lucide-react';

export default function Login({ setToken }) {
  const [isRegistering, setIsRegistering] = useState(false);
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [success, setSuccess] = useState('');

  const handleSubmit = async (e) => {
    e.preventDefault();
    setError('');
    setSuccess('');
    try {
      if (isRegistering) {
        await axios.post('http://localhost:3000/api/register', { username, password });
        setSuccess('Registration successful! Please sign in.');
        setIsRegistering(false);
        setPassword('');
      } else {
        const res = await axios.post('http://localhost:3000/api/login', { username, password });
        setToken(res.data.token);
      }
    } catch (err) {
      setError(err.response?.data?.error || (isRegistering ? 'Registration failed' : 'Invalid credentials'));
    }
  };

  return (
    <div className="flex items-center justify-center min-h-screen">
      <div className="bg-darker p-8 rounded-2xl border border-slate-800 shadow-2xl w-full max-w-md">
        <div className="flex justify-center mb-6 text-primary">
          {isRegistering ? <UserPlus size={48} /> : <KeyRound size={48} />}
        </div>
        <h2 className="text-3xl font-bold text-center mb-8 text-white">
          {isRegistering ? 'Create Account' : 'Omni-Tracker'}
        </h2>
        {error && <div className="bg-red-500/10 border border-red-500/50 text-red-400 p-3 rounded-lg mb-6 text-center text-sm">{error}</div>}
        {success && <div className="bg-green-500/10 border border-green-500/50 text-green-400 p-3 rounded-lg mb-6 text-center text-sm">{success}</div>}
        
        <form onSubmit={handleSubmit} className="space-y-6">
          <div>
            <label className="block text-sm font-medium text-slate-400 mb-2">Username</label>
            <input type="text" value={username} onChange={e => setUsername(e.target.value)}
              className="w-full bg-slate-900 border border-slate-700 rounded-lg p-3 text-white focus:ring-2 focus:ring-primary outline-none transition-all" required minLength="3" />
          </div>
          <div>
            <label className="block text-sm font-medium text-slate-400 mb-2">Password</label>
            <input type="password" value={password} onChange={e => setPassword(e.target.value)}
              className="w-full bg-slate-900 border border-slate-700 rounded-lg p-3 text-white focus:ring-2 focus:ring-primary outline-none transition-all" required minLength="6" />
          </div>
          <button type="submit" className="w-full bg-gradient-to-r from-primary to-secondary hover:opacity-90 text-white font-bold py-3 rounded-lg transition-all shadow-lg shadow-primary/30">
            {isRegistering ? 'Sign Up' : 'Sign In'}
          </button>
        </form>
        
        <div className="mt-6 text-center">
          <button onClick={() => { setIsRegistering(!isRegistering); setError(''); setSuccess(''); }} className="text-sm text-slate-400 hover:text-primary transition-colors cursor-pointer">
            {isRegistering ? 'Already have an account? Sign In' : "Don't have an account? Sign Up"}
          </button>
        </div>
      </div>
    </div>
  );
}
