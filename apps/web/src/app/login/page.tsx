'use client';

import React, { useState, useEffect } from 'react';
import { useRouter } from 'next/navigation';
import { Cpu, ShieldCheck, UserCheck, AlertCircle } from 'lucide-react';
import { DEFAULT_DEV_TOKEN, PlantBrainSession, setStoredSession } from '../../lib/api';

export default function LoginPage() {
  const router = useRouter();
  const [isLogin, setIsLogin] = useState(true);
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [name, setName] = useState('');
  const [role, setRole] = useState('Maintenance Engineer');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    // If user is already logged in, redirect to dashboard
    if (localStorage.getItem('plantbrain_session')) {
      router.push('/');
    }
  }, [router]);

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    setError('');
    setLoading(true);

    if (isLogin) {
      // Simulate Better Auth Login
      if (!email || !password) {
        setError('Please enter email and password');
        setLoading(false);
        return;
      }
      
      // Setup demo session based on email keywords or defaults
      let userRole = 'Plant Operator';
      let userName = 'Operator User';
      
      if (email.includes('arjun') || email.includes('eng')) {
        userName = 'Arjun Dev';
        userRole = 'Maintenance Engineer';
      } else if (email.includes('meera') || email.includes('comp')) {
        userName = 'Meera Sharma';
        userRole = 'Compliance Officer';
      } else if (email.includes('suresh') || email.includes('mgr')) {
        userName = 'Suresh Nair';
        userRole = 'Plant Manager';
      } else if (email.includes('ravi') || email.includes('tech')) {
        userName = 'Ravi Kumar';
        userRole = 'Field Technician';
      } else {
        userName = email.split('@')[0];
        // Capitalize first letter
        userName = userName.charAt(0).toUpperCase() + userName.slice(1);
      }

      const token = email.includes('meera') || email.includes('comp')
        ? 'meera-token'
        : email.includes('suresh') || email.includes('mgr')
          ? 'suresh-token'
          : email.includes('ravi') || email.includes('tech')
            ? 'ravi-token'
            : DEFAULT_DEV_TOKEN;

      setTimeout(() => {
        setStoredSession({
          name: userName,
          role: userRole,
          email,
          token,
          orgId: 'org_demo',
          organizationName: 'PlantBrain Demo Works',
          plantId: 'plant_unit_2',
          plantName: 'Unit-2 Primary Feed',
        });
        router.push('/');
      }, 1000);
    } else {
      // Simulate Better Auth Registration
      if (!email || !password || !name) {
        setError('All fields are required');
        setLoading(false);
        return;
      }

      setTimeout(() => {
        setStoredSession({
          name,
          role,
          email,
          token: DEFAULT_DEV_TOKEN,
          orgId: 'org_demo',
          organizationName: 'PlantBrain Demo Works',
          plantId: 'plant_unit_2',
          plantName: 'Unit-2 Primary Feed',
        });
        router.push('/');
      }, 1000);
    }
  };

  const handleQuickLogin = (demoUser: PlantBrainSession) => {
    setLoading(true);
    setTimeout(() => {
      setStoredSession(demoUser);
      router.push('/');
    }, 600);
  };

  const demoAccounts: PlantBrainSession[] = [
    { name: 'Arjun Dev', role: 'Maintenance Engineer', email: 'arjun@plantbrain.ai', token: DEFAULT_DEV_TOKEN, orgId: 'org_demo', organizationName: 'PlantBrain Demo Works', plantId: 'plant_unit_2', plantName: 'Unit-2 Primary Feed' },
    { name: 'Ravi Kumar', role: 'Field Technician', email: 'ravi@plantbrain.ai', token: 'ravi-token', orgId: 'org_demo', organizationName: 'PlantBrain Demo Works', plantId: 'plant_unit_2', plantName: 'Unit-2 Primary Feed' },
    { name: 'Meera Sharma', role: 'Compliance Officer', email: 'meera@plantbrain.ai', token: 'meera-token', orgId: 'org_demo', organizationName: 'PlantBrain Demo Works', plantId: 'plant_unit_2', plantName: 'Unit-2 Primary Feed' },
    { name: 'Suresh Nair', role: 'Plant Manager', email: 'suresh@plantbrain.ai', token: 'suresh-token', orgId: 'org_demo', organizationName: 'PlantBrain Demo Works', plantId: 'plant_unit_2', plantName: 'Unit-2 Primary Feed' },
  ];

  return (
    <div className="flex flex-col items-center justify-center min-h-screen bg-[#020617] px-4 py-12 relative overflow-hidden">
      {/* Background Decorative Rings */}
      <div className="absolute top-1/4 left-1/4 w-96 h-96 bg-cyber-emerald/5 rounded-full blur-[100px] pointer-events-none" />
      <div className="absolute bottom-1/4 right-1/4 w-96 h-96 bg-cyber-indigo/5 rounded-full blur-[100px] pointer-events-none" />

      {/* Brand Header */}
      <div className="flex flex-col items-center gap-3 mb-8">
        <div className="bg-gradient-to-tr from-cyber-emerald to-cyber-blue p-3.5 rounded-2xl shadow-[0_0_30px_rgba(16,185,129,0.3)] border border-emerald-400/20">
          <Cpu className="w-8 h-8 text-white animate-pulse" />
        </div>
        <div className="text-center">
          <h1 className="text-3xl font-extrabold text-white tracking-wider">
            PlantBrain<span className="text-cyber-emerald">AI</span>
          </h1>
          <p className="text-xs text-slate-400 font-mono tracking-widest uppercase mt-1">
            Unified Asset & Operations Brain
          </p>
        </div>
      </div>

      {/* Main Authentication Card */}
      <div className="w-full max-w-md glass-panel p-8 rounded-2xl border-slate-800/80 shadow-[0_20px_50px_rgba(0,0,0,0.5)]">
        {/* Tabs */}
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

        {/* Credentials Form */}
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
            <div>
              <label className="block text-xs font-semibold text-slate-400 uppercase tracking-wider mb-1.5">Plant Role</label>
              <select
                value={role}
                onChange={(e) => setRole(e.target.value)}
                className="w-full px-4 py-3 rounded-xl glass-input text-sm font-medium"
              >
                <option value="Maintenance Engineer">Maintenance Engineer</option>
                <option value="Compliance Officer">Compliance Officer</option>
                <option value="Plant Manager">Plant Manager</option>
                <option value="Plant Operator">Plant Operator</option>
                <option value="Reliability Engineer">Reliability Engineer</option>
                <option value="Field Technician">Field Technician</option>
              </select>
            </div>
          )}

          <button
            type="submit"
            disabled={loading}
            className="w-full py-3 bg-cyber-emerald hover:bg-emerald-600 active:bg-emerald-700 text-slate-950 font-bold rounded-xl text-sm transition-all duration-200 mt-2 hover:shadow-[0_0_20px_rgba(16,185,129,0.3)] disabled:opacity-50"
          >
            {loading ? 'Authenticating...' : isLogin ? 'Access Brain' : 'Register Operator'}
          </button>
        </form>

        {/* Quick Demo Login Option */}
        {isLogin && (
          <div className="mt-8 border-t border-slate-800/80 pt-6">
            <p className="text-center text-[10px] text-slate-500 font-mono tracking-wider uppercase mb-3.5">
              Or Sign In with Demo Profile
            </p>
            <div className="grid grid-cols-1 gap-2">
              {demoAccounts.map((account) => (
                <button
                  key={account.email}
                  onClick={() => handleQuickLogin(account)}
                  disabled={loading}
                  className="flex items-center justify-between px-4 py-2.5 rounded-xl border border-slate-800 hover:border-emerald-500/30 bg-[#060918]/60 hover:bg-emerald-500/5 text-left text-xs font-medium text-slate-300 transition-all hover:-translate-y-0.5 duration-200"
                >
                  <div className="flex items-center gap-2">
                    <UserCheck className="w-3.5 h-3.5 text-cyber-emerald" />
                    <div>
                      <span className="font-semibold text-slate-200 block">{account.name}</span>
                      <span className="text-[10px] text-slate-500 font-mono">{account.role}</span>
                    </div>
                  </div>
                  <span className="text-[10px] font-mono text-slate-500">{account.email.split('@')[0]}</span>
                </button>
              ))}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
