'use client';

import React, { useEffect, useState } from 'react';
import NavigationShell from '../../components/NavigationShell';
import { apiFetch } from '../../lib/api';
import { CheckCircle2, Database, ShieldCheck, UserCheck } from 'lucide-react';

const roles = [
  { name: 'Admin', scope: 'Users, documents, settings' },
  { name: 'Plant Manager', scope: 'Dashboard, reports, risks' },
  { name: 'Engineer', scope: 'Documents, assets, RCA, Copilot' },
  { name: 'Technician', scope: 'SOPs, assets, Copilot' },
  { name: 'Compliance Officer', scope: 'Compliance gaps, reports' },
  { name: 'Viewer', scope: 'Read-only access' },
];

export default function AdminSettingsPage() {
  const [session, setSession] = useState<{ userId?: string; orgId?: string; plantId?: string; role?: string } | null>(null);

  useEffect(() => {
    apiFetch('/api/me')
      .then(res => (res.ok ? res.json() : null))
      .then(data => setSession(data))
      .catch(() => setSession(null));
  }, []);

  return (
    <NavigationShell>
      <div className="space-y-6">
        <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
          <div className="glass-panel p-5 rounded-2xl border-slate-800/80">
            <div className="flex items-center gap-3">
              <div className="p-3 bg-emerald-500/10 text-cyber-emerald rounded-xl border border-emerald-500/20">
                <ShieldCheck className="w-5 h-5" />
              </div>
              <div>
                <span className="text-[10px] text-slate-500 font-mono uppercase tracking-wider block">Session Role</span>
                <span className="text-lg font-bold text-white">{session?.role || 'Demo'}</span>
              </div>
            </div>
          </div>

          <div className="glass-panel p-5 rounded-2xl border-slate-800/80">
            <div className="flex items-center gap-3">
              <div className="p-3 bg-indigo-500/10 text-cyber-indigo rounded-xl border border-indigo-500/20">
                <Database className="w-5 h-5" />
              </div>
              <div>
                <span className="text-[10px] text-slate-500 font-mono uppercase tracking-wider block">Plant Context</span>
                <span className="text-lg font-bold text-white">{session?.plantId ? session.plantId.slice(0, 8) : 'Offline'}</span>
              </div>
            </div>
          </div>

          <div className="glass-panel p-5 rounded-2xl border-slate-800/80">
            <div className="flex items-center gap-3">
              <div className="p-3 bg-cyan-500/10 text-cyan-400 rounded-xl border border-cyan-500/20">
                <UserCheck className="w-5 h-5" />
              </div>
              <div>
                <span className="text-[10px] text-slate-500 font-mono uppercase tracking-wider block">Auth Status</span>
                <span className="text-lg font-bold text-white">{session?.userId ? 'Verified' : 'Local'}</span>
              </div>
            </div>
          </div>
        </div>

        <div className="glass-panel p-6 rounded-2xl border-slate-800/80">
          <div className="flex items-center justify-between mb-5">
            <div>
              <h3 className="text-base font-bold text-white tracking-wide">Role Matrix</h3>
              <p className="text-xs text-slate-400 mt-0.5">Workspace access model</p>
            </div>
            <span className="text-[10px] bg-cyber-emerald/10 text-cyber-emerald border border-cyber-emerald/20 px-2.5 py-1 rounded-full font-mono font-bold flex items-center gap-1">
              <CheckCircle2 className="w-3 h-3" /> Active
            </span>
          </div>

          <div className="overflow-x-auto">
            <table className="w-full text-left border-collapse text-xs">
              <thead>
                <tr className="border-b border-slate-800/80 text-[10px] font-mono text-slate-500 uppercase tracking-widest">
                  <th className="py-3 px-4">Role</th>
                  <th className="py-3 px-4">Scope</th>
                  <th className="py-3 px-4 text-center">Status</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-800/40">
                {roles.map(role => (
                  <tr key={role.name} className="hover:bg-slate-800/10">
                    <td className="py-4 px-4 font-bold text-slate-200">{role.name}</td>
                    <td className="py-4 px-4 text-slate-400">{role.scope}</td>
                    <td className="py-4 px-4 text-center">
                      <span className="text-[10px] bg-slate-900 text-slate-300 border border-slate-800 px-2.5 py-0.5 rounded-full font-mono font-bold">
                        Enabled
                      </span>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </NavigationShell>
  );
}
