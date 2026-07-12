'use client';

import React, { useState, useEffect } from 'react';
import { useRouter } from 'next/navigation';
import { Cpu, AlertCircle } from 'lucide-react';
import { apiFetch, getStoredSession, setStoredSession } from '../../lib/api';
import { authClient } from '../../lib/auth-client';

export default function LoginPage() {
  const router = useRouter();
  const [isLogin, setIsLogin] = useState(true);
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [name, setName] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (getStoredSession()) {
      router.push('/');
    }
  }, [router]);

  // After BetterAuth sets the session cookie, resolve the real role/org/plant from the
  // gateway and cache it (display only — the cookie is the source of truth for auth).
  const bootstrapSession = async (fallbackName: string) => {
    let role = 'engineer';
    let userId: string | undefined;
    let orgId: string | undefined;
    let plantId: string | undefined;
    try {
      const meRes = await apiFetch('/api/me');
      if (meRes.ok) {
        const me = await meRes.json();
        role = me.role || role;
        userId = me.userId;
        orgId = me.orgId;
        plantId = me.plantId;
      }
    } catch {
      // fall through with defaults
    }

    let displayName = fallbackName;
    try {
      const session = await authClient.getSession();
      displayName = session.data?.user?.name || fallbackName;
    } catch {
      // ignore
    }

    setStoredSession({ name: displayName || email.split('@')[0], email, role, userId, orgId, plantId });
    router.push('/');
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');

    if (!email || !password || (!isLogin && !name)) {
      setError(isLogin ? 'Please enter email and password' : 'All fields are required');
      return;
    }

    setLoading(true);
    try {
      if (isLogin) {
        const { error: authError } = await authClient.signIn.email({ email, password });
        if (authError) {
          setError(authError.message || 'Invalid email or password');
          setLoading(false);
          return;
        }
        await bootstrapSession(email.split('@')[0]);
      } else {
        const { error: authError } = await authClient.signUp.email({ email, password, name });
        if (authError) {
          setError(authError.message || 'Could not create account');
          setLoading(false);
          return;
        }
        await bootstrapSession(name);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Authentication failed. Is the backend running?');
      setLoading(false);
    }
  };

  return (
    <div className="flex flex-col items-center justify-center min-h-screen bg-[#020617] px-4 py-12 relative overflow-hidden">
      <div className="absolute top-1/4 left-1/4 w-96 h-96 bg-cyber-emerald/5 rounded-full blur-[100px] pointer-events-none" />
      <div className="absolute bottom-1/4 right-1/4 w-96 h-96 bg-cyber-indigo/5 rounded-full blur-[100px] pointer-events-none" />

      <div className="flex flex-col items-center gap-3 mb-8">
        <div className="bg-gradient-to-tr from-cyber-emerald to-cyber-blue p-3.5 rounded-2xl shadow-[0_0_30px_rgba(16,185,129,0.3)] border border-emerald-400/20">
          <Cpu className="w-8 h-8 text-white animate-pulse" />
        </div>
        <div className="text-center">
          <h1 className="text-3xl font-extrabold text-white tracking-wider">
            PlantBrain<span className="text-cyber-emerald">AI</span>
          </h1>
          <p className="text-xs text-slate-400 font-mono tracking-widest uppercase mt-1">
            Unified Asset &amp; Operations Brain
          </p>
        </div>
      </div>

      <div className="w-full max-w-md glass-panel p-8 rounded-2xl border-slate-800/80 shadow-[0_20px_50px_rgba(0,0,0,0.5)]">
        <div className="flex border-b border-slate-800 pb-4 mb-6">
          <button
            onClick={() => { setIsLogin(true); setError(''); }}
            className={`flex-1 text-center font-semibold text-sm pb-2 transition-all ${
              isLogin ? 'text-cyber-emerald border-b-2 border-cyber-emerald' : 'text-slate-500 hover:text-slate-300'
            }`}
          >
            Sign In
          </button>
          <button
            onClick={() => { setIsLogin(false); setError(''); }}
            className={`flex-1 text-center font-semibold text-sm pb-2 transition-all ${
              !isLogin ? 'text-cyber-emerald border-b-2 border-cyber-emerald' : 'text-slate-500 hover:text-slate-300'
            }`}
          >
            Create Account
          </button>
        </div>

        {error && (
          <div className="bg-cyber-rose/10 border border-cyber-rose/20 text-rose-300 p-3.5 rounded-xl mb-5 flex items-start gap-2.5 text-xs">
            <AlertCircle className="w-4 h-4 shrink-0 mt-0.5" />
            <span>{error}</span>
          </div>
        )}

        <form onSubmit={handleSubmit} className="space-y-4">
          {!isLogin && (
            <div>
              <label className="block text-xs font-semibold text-slate-400 uppercase tracking-wider mb-1.5">Full Name</label>
              <input
                type="text"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="e.g. Arjun Dev"
                className="w-full px-4 py-3 rounded-xl glass-input text-sm font-medium"
              />
            </div>
          )}

          <div>
            <label className="block text-xs font-semibold text-slate-400 uppercase tracking-wider mb-1.5">Corporate Email</label>
            <input
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              placeholder="name@company.com"
              className="w-full px-4 py-3 rounded-xl glass-input text-sm font-medium"
            />
          </div>

          <div>
            <label className="block text-xs font-semibold text-slate-400 uppercase tracking-wider mb-1.5">Password</label>
            <input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder="••••••••"
              className="w-full px-4 py-3 rounded-xl glass-input text-sm font-medium"
            />
          </div>

          {!isLogin && (
            <p className="text-[10px] text-slate-500 font-mono">
              New accounts join the default plant workspace with the Engineer role. An admin can adjust roles afterward.
            </p>
          )}

          <button
            type="submit"
            disabled={loading}
            className="w-full py-3 bg-cyber-emerald hover:bg-emerald-600 active:bg-emerald-700 text-slate-950 font-bold rounded-xl text-sm transition-all duration-200 mt-2 hover:shadow-[0_0_20px_rgba(16,185,129,0.3)] disabled:opacity-50"
          >
            {loading ? 'Authenticating...' : isLogin ? 'Access Brain' : 'Register Operator'}
          </button>
        </form>
      </div>
    </div>
  );
}
