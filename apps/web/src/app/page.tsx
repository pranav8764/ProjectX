'use client';

import React, { useState, useEffect } from 'react';
import NavigationShell from '../components/NavigationShell';
import { useData } from '../context/DataContext';
import { 
  FileText, 
  Layers, 
  ShieldAlert, 
  Award,
  TrendingUp, 
  Activity, 
  ArrowUpRight,
  Clock,
  CheckCircle2,
  AlertTriangle,
  ChevronRight
} from 'lucide-react';
import Link from 'next/link';
import { 
  BarChart, 
  Bar, 
  XAxis, 
  YAxis, 
  Tooltip, 
  ResponsiveContainer, 
  AreaChart, 
  Area,
  PieChart,
  Pie,
  Cell
} from 'recharts';

export default function DashboardPage() {
  const { documents, assets, complianceGaps, certificates } = useData();
  const [mounted, setMounted] = useState(false);

  useEffect(() => {
    setMounted(true);
  }, []);

  // Compute stats
  const totalDocs = documents.length;
  const totalAssets = assets.length;
  const openGaps = complianceGaps.filter(g => g.status === 'Open').length;
  const expiredCerts = certificates.filter(c => c.status === 'Expired' || c.status === 'Overdue').length;

  // Calculate average risk score
  const avgRisk = Math.round(assets.reduce((acc, curr) => acc + curr.riskScore, 0) / (totalAssets || 1));

  // Count active uploads in progress
  const processingDocsCount = documents.filter(d => d.status !== 'COMPLETED' && d.status !== 'FAILED').length;

  // Prepare chart data: Asset Risks
  const assetChartData = assets.map(a => ({
    name: a.assetTag,
    risk: a.riskScore,
    criticality: a.criticality
  })).sort((a, b) => b.risk - a.risk);

  // Prepare chart data: Document Types
  const docTypes = documents.reduce((acc: { [key: string]: number }, doc) => {
    acc[doc.documentType] = (acc[doc.documentType] || 0) + 1;
    return acc;
  }, {});

  const docPieData = Object.keys(docTypes).map(key => ({
    name: key,
    value: docTypes[key]
  }));

  const COLORS = ['#10b981', '#3b82f6', '#6366f1', '#f59e0b', '#f43f5e', '#8b5cf6'];

  return (
    <NavigationShell>
      <div className="space-y-8">
        
        {/* Metric Cards Row */}
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6">
          
          {/* Total Documents */}
          <div className="glass-panel hover:border-emerald-500/20 transition-all duration-300 p-6 rounded-2xl group relative overflow-hidden">
            <div className="absolute top-0 right-0 w-24 h-24 bg-cyber-emerald/5 rounded-full blur-2xl pointer-events-none" />
            <div className="flex items-center justify-between mb-4">
              <div className="p-3 bg-emerald-500/10 text-cyber-emerald rounded-xl border border-emerald-500/20">
                <FileText className="w-6 h-6" />
              </div>
              <Link href="/documents" className="text-slate-500 hover:text-cyber-emerald transition-colors">
                <ArrowUpRight className="w-4 h-4" />
              </Link>
            </div>
            <span className="text-slate-400 font-mono text-xs uppercase tracking-wider block">Ingested Documents</span>
            <div className="flex items-baseline gap-2 mt-1">
              <span className="text-3xl font-extrabold text-white">{totalDocs}</span>
              {processingDocsCount > 0 && (
                <span className="text-[10px] bg-cyber-amber/10 text-cyber-amber border border-cyber-amber/20 px-2 py-0.5 rounded-full font-mono animate-pulse">
                  {processingDocsCount} processing
                </span>
              )}
            </div>
            <p className="text-[10px] text-slate-500 font-mono mt-3">OCR text parsing & embeddings active</p>
          </div>

          {/* Discovered Assets */}
          <div className="glass-panel hover:border-indigo-500/20 transition-all duration-300 p-6 rounded-2xl group relative overflow-hidden">
            <div className="absolute top-0 right-0 w-24 h-24 bg-cyber-indigo/5 rounded-full blur-2xl pointer-events-none" />
            <div className="flex items-center justify-between mb-4">
              <div className="p-3 bg-indigo-500/10 text-cyber-indigo rounded-xl border border-indigo-500/20">
                <Layers className="w-6 h-6" />
              </div>
              <Link href="/assets" className="text-slate-500 hover:text-cyber-indigo transition-colors">
                <ArrowUpRight className="w-4 h-4" />
              </Link>
            </div>
            <span className="text-slate-400 font-mono text-xs uppercase tracking-wider block">Discovered Assets</span>
            <div className="flex items-baseline gap-2 mt-1">
              <span className="text-3xl font-extrabold text-white">{totalAssets}</span>
              <span className="text-[10px] bg-cyber-emerald/10 text-cyber-emerald border border-cyber-emerald/20 px-2 py-0.5 rounded-full font-mono">
                100% linked
              </span>
            </div>
            <p className="text-[10px] text-slate-500 font-mono mt-3">Mapped via Knowledge Graph relationships</p>
          </div>

          {/* Open Compliance Gaps */}
          <div className="glass-panel hover:border-rose-500/20 transition-all duration-300 p-6 rounded-2xl group relative overflow-hidden">
            <div className="absolute top-0 right-0 w-24 h-24 bg-cyber-rose/5 rounded-full blur-2xl pointer-events-none" />
            <div className="flex items-center justify-between mb-4">
              <div className="p-3 bg-rose-500/10 text-cyber-rose rounded-xl border border-rose-500/20">
                <ShieldAlert className="w-6 h-6" />
              </div>
              <Link href="/compliance" className="text-slate-500 hover:text-cyber-rose transition-colors">
                <ArrowUpRight className="w-4 h-4" />
              </Link>
            </div>
            <span className="text-slate-400 font-mono text-xs uppercase tracking-wider block">Compliance Gaps</span>
            <div className="flex items-baseline gap-2 mt-1">
              <span className="text-3xl font-extrabold text-white">{openGaps}</span>
              {openGaps > 0 && (
                <span className="text-[10px] bg-cyber-rose/10 text-cyber-rose border border-cyber-rose/20 px-2 py-0.5 rounded-full font-mono">
                  Needs Attention
                </span>
              )}
            </div>
            <p className="text-[10px] text-slate-500 font-mono mt-3">Expired tests or missing logs flagged</p>
          </div>

          {/* Expired Certificates */}
          <div className="glass-panel hover:border-amber-500/20 transition-all duration-300 p-6 rounded-2xl group relative overflow-hidden">
            <div className="absolute top-0 right-0 w-24 h-24 bg-cyber-amber/5 rounded-full blur-2xl pointer-events-none" />
            <div className="flex items-center justify-between mb-4">
              <div className="p-3 bg-amber-500/10 text-cyber-amber rounded-xl border border-cyber-amber/20">
                <Award className="w-6 h-6" />
              </div>
              <Link href="/compliance" className="text-slate-500 hover:text-cyber-amber transition-colors">
                <ArrowUpRight className="w-4 h-4" />
              </Link>
            </div>
            <span className="text-slate-400 font-mono text-xs uppercase tracking-wider block">Expired Certificates</span>
            <div className="flex items-baseline gap-2 mt-1">
              <span className="text-3xl font-extrabold text-white">{expiredCerts}</span>
              <span className="text-[10px] bg-amber-500/10 text-cyber-amber border border-cyber-amber/20 px-2 py-0.5 rounded-full font-mono">
                {certificates.filter(c => c.status === 'Active').length} active
              </span>
            </div>
            <p className="text-[10px] text-slate-500 font-mono mt-3">Regulatory inspection compliance rate</p>
          </div>

        </div>

        {/* Charts & Interactive Section */}
        <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
          
          {/* Main Chart Area */}
          <div className="glass-panel p-6 rounded-2xl lg:col-span-2 flex flex-col justify-between">
            <div>
              <div className="flex items-center justify-between mb-6">
                <div>
                  <h3 className="text-base font-bold text-white tracking-wide">Asset Risk Index Distribution</h3>
                  <p className="text-xs text-slate-400 mt-1">Active risk evaluation score based on failure logs and gaps</p>
                </div>
                <div className="bg-slate-800/80 px-3 py-1.5 rounded-xl border border-slate-700 font-mono text-xs">
                  Avg Risk: <span className="text-cyber-amber font-extrabold">{avgRisk} / 100</span>
                </div>
              </div>

              {/* Chart container */}
              <div className="h-64 w-full">
                {mounted ? (
                  <ResponsiveContainer width="100%" height="100%">
                    <BarChart data={assetChartData}>
                      <defs>
                        <linearGradient id="colorRisk" x1="0" y1="0" x2="0" y2="1">
                          <stop offset="5%" stopColor="#10b981" stopOpacity={0.8}/>
                          <stop offset="95%" stopColor="#6366f1" stopOpacity={0.1}/>
                        </linearGradient>
                      </defs>
                      <XAxis dataKey="name" stroke="#64748b" fontSize={11} tickLine={false} />
                      <YAxis stroke="#64748b" fontSize={11} tickLine={false} domain={[0, 100]} />
                      <Tooltip 
                        contentStyle={{ 
                          backgroundColor: 'rgba(9, 13, 31, 0.95)', 
                          borderColor: 'rgba(148, 163, 184, 0.2)',
                          borderRadius: '12px',
                          color: '#fff',
                          fontFamily: 'Inter, sans-serif',
                          fontSize: '12px'
                        }}
                      />
                      <Bar dataKey="risk" fill="url(#colorRisk)" radius={[6, 6, 0, 0]} barSize={36} />
                    </BarChart>
                  </ResponsiveContainer>
                ) : (
                  <div className="w-full h-full flex items-center justify-center bg-slate-900/20 rounded-xl">
                    <span className="text-xs text-slate-500 font-mono">Loading telemetry graph...</span>
                  </div>
                )}
              </div>
            </div>
            
            <div className="border-t border-slate-800/80 pt-4 mt-4 flex items-center justify-between text-xs text-slate-400">
              <div className="flex items-center gap-4">
                <span className="flex items-center gap-1.5"><span className="w-2.5 h-2.5 rounded bg-cyber-emerald" /> P-101 (Critical Risk: 78)</span>
                <span className="flex items-center gap-1.5"><span className="w-2.5 h-2.5 rounded bg-cyber-indigo" /> B-12 (High Risk: 65)</span>
              </div>
              <Link href="/assets" className="text-cyber-emerald hover:underline flex items-center gap-0.5">
                Explore assets <ChevronRight className="w-3.5 h-3.5" />
              </Link>
            </div>
          </div>

          {/* Doc breakdown and status pie */}
          <div className="glass-panel p-6 rounded-2xl flex flex-col justify-between">
            <div>
              <h3 className="text-base font-bold text-white tracking-wide mb-6">Knowledge Base Composition</h3>
              
              <div className="h-48 w-full flex items-center justify-center">
                {mounted ? (
                  <ResponsiveContainer width="100%" height="100%">
                    <PieChart>
                      <Pie
                        data={docPieData}
                        cx="50%"
                        cy="50%"
                        innerRadius={60}
                        outerRadius={80}
                        paddingAngle={3}
                        dataKey="value"
                      >
                        {docPieData.map((entry, index) => (
                          <Cell key={`cell-${index}`} fill={COLORS[index % COLORS.length]} />
                        ))}
                      </Pie>
                      <Tooltip
                        contentStyle={{
                          backgroundColor: 'rgba(9, 13, 31, 0.95)',
                          borderColor: 'rgba(148, 163, 184, 0.2)',
                          borderRadius: '12px',
                          color: '#fff',
                          fontFamily: 'Inter, sans-serif',
                          fontSize: '11px'
                        }}
                      />
                    </PieChart>
                  </ResponsiveContainer>
                ) : (
                  <div className="w-full h-full flex items-center justify-center bg-slate-900/20 rounded-xl">
                    <span className="text-xs text-slate-500 font-mono">Loading schema breakdown...</span>
                  </div>
                )}
              </div>
            </div>

            {/* List labels */}
            <div className="space-y-2 mt-4 max-h-[160px] overflow-y-auto pr-1">
              {docPieData.map((item, idx) => (
                <div key={item.name} className="flex items-center justify-between text-xs">
                  <div className="flex items-center gap-2">
                    <span className="w-2.5 h-2.5 rounded-full" style={{ backgroundColor: COLORS[idx % COLORS.length] }} />
                    <span className="text-slate-300 font-medium">{item.name}</span>
                  </div>
                  <span className="text-slate-400 font-mono font-semibold">{item.value} files</span>
                </div>
              ))}
            </div>
          </div>

        </div>

        {/* Bottom grid: Processing Monitor & Critical Issues */}
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
          
          {/* Document ingestion stream monitor */}
          <div className="glass-panel p-6 rounded-2xl">
            <div className="flex items-center justify-between mb-4">
              <div>
                <h3 className="text-base font-bold text-white tracking-wide">Document Ingestion Pipeline</h3>
                <p className="text-xs text-slate-400 mt-0.5">Real-time tracking of extracted files</p>
              </div>
              <Link href="/documents" className="text-xs text-cyber-emerald hover:underline">
                Upload Center
              </Link>
            </div>

            <div className="space-y-3 max-h-[280px] overflow-y-auto pr-1">
              {documents.slice(0, 5).map((doc) => {
                const isProcessing = doc.status !== 'COMPLETED' && doc.status !== 'FAILED';
                return (
                  <div 
                    key={doc.id}
                    className="flex items-center justify-between p-3 bg-[#060918]/60 border border-slate-800/80 hover:border-slate-700/50 rounded-xl transition-all"
                  >
                    <div className="flex items-center gap-3 min-w-0">
                      <FileText className={`w-5 h-5 shrink-0 ${isProcessing ? 'text-cyber-amber animate-pulse' : 'text-cyber-blue'}`} />
                      <div className="min-w-0">
                        <span className="text-xs font-semibold text-slate-200 block truncate">{doc.title}</span>
                        <span className="text-[10px] text-slate-500 font-mono uppercase tracking-wider">{doc.documentType} • {doc.size}</span>
                      </div>
                    </div>
                    
                    <div>
                      {doc.status === 'COMPLETED' ? (
                        <span className="text-[10px] bg-cyber-emerald/10 text-cyber-emerald border border-cyber-emerald/20 px-2.5 py-1 rounded-full font-mono font-bold flex items-center gap-1">
                          <CheckCircle2 className="w-3 h-3" /> Completed
                        </span>
                      ) : doc.status === 'FAILED' ? (
                        <span className="text-[10px] bg-cyber-rose/10 text-cyber-rose border border-cyber-rose/20 px-2.5 py-1 rounded-full font-mono font-bold">
                          Failed
                        </span>
                      ) : (
                        <span className="text-[10px] bg-cyber-amber/10 text-cyber-amber border border-cyber-amber/20 px-2.5 py-1 rounded-full font-mono font-semibold animate-pulse-glow">
                          {doc.status.replace('_', ' ')}
                        </span>
                      )}
                    </div>
                  </div>
                );
              })}
            </div>
          </div>

          {/* Active compliance issues & warnings */}
          <div className="glass-panel p-6 rounded-2xl">
            <div className="flex items-center justify-between mb-4">
              <div>
                <h3 className="text-base font-bold text-white tracking-wide">Critical Gaps & Open Failures</h3>
                <p className="text-xs text-slate-400 mt-0.5">Urgent compliance risks and active asset malfunctions</p>
              </div>
              <Link href="/compliance" className="text-xs text-cyber-rose hover:underline">
                View Audits
              </Link>
            </div>

            <div className="space-y-3 max-h-[280px] overflow-y-auto pr-1">
              {/* Combine Gaps & Open Failures */}
              {complianceGaps.filter(g => g.status === 'Open').slice(0, 4).map((gap) => (
                <div 
                  key={gap.id}
                  className="p-3 bg-[#060918]/60 border border-slate-800/80 hover:border-slate-700/50 rounded-xl flex items-start justify-between gap-4"
                >
                  <div className="flex gap-2.5 min-w-0">
                    <AlertTriangle className={`w-4 h-4 shrink-0 mt-0.5 ${
                      gap.severity === 'Critical' ? 'text-cyber-rose' :
                      gap.severity === 'High' ? 'text-cyber-amber' : 'text-cyan-400'
                    }`} />
                    <div className="min-w-0">
                      <div className="flex items-center gap-2">
                        <span className="text-xs font-bold text-slate-200">{gap.gapType}</span>
                        <span className="text-[9px] bg-slate-800 text-slate-400 border border-slate-700 px-1.5 py-0.5 rounded font-mono font-bold uppercase">{gap.assetTag}</span>
                      </div>
                      <p className="text-[11px] text-slate-400 mt-1 truncate">{gap.description}</p>
                    </div>
                  </div>
                  <span className={`text-[9px] font-mono font-bold uppercase border px-2 py-0.5 rounded-full shrink-0 ${
                    gap.severity === 'Critical' ? 'bg-cyber-rose/10 text-cyber-rose border-cyber-rose/20' :
                    gap.severity === 'High' ? 'bg-cyber-amber/10 text-cyber-amber border-cyber-amber/20' : 'bg-cyan-500/10 text-cyan-400 border-cyan-500/20'
                  }`}>
                    {gap.severity}
                  </span>
                </div>
              ))}
            </div>
          </div>

        </div>

      </div>
    </NavigationShell>
  );
}
