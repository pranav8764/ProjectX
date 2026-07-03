'use client';

import React, { useState } from 'react';
import NavigationShell from '../../components/NavigationShell';
import { useData } from '../../context/DataContext';
import { 
  Search, 
  Filter, 
  Wrench, 
  AlertTriangle,
  ArrowRight,
  TrendingUp,
  Activity,
  ShieldAlert,
  FileText
} from 'lucide-react';
import Link from 'next/link';

export default function AssetExplorerPage() {
  const { assets } = useData();
  const [searchQuery, setSearchQuery] = useState('');
  const [criticalityFilter, setCriticalityFilter] = useState('All');
  const [typeFilter, setTypeFilter] = useState('All');

  // Gather types
  const assetTypes = Array.from(new Set(assets.map(a => a.assetType)));

  // Filter logic
  const filteredAssets = assets.filter(asset => {
    const matchesSearch = asset.assetTag.toLowerCase().includes(searchQuery.toLowerCase()) || 
                          asset.assetName.toLowerCase().includes(searchQuery.toLowerCase());
    const matchesCriticality = criticalityFilter === 'All' || asset.criticality === criticalityFilter;
    const matchesType = typeFilter === 'All' || asset.assetType === typeFilter;
    return matchesSearch && matchesCriticality && matchesType;
  });

  return (
    <NavigationShell>
      <div className="space-y-6">
        
        {/* Header Summary */}
        <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
          <div className="glass-panel p-4 rounded-xl flex items-center gap-4">
            <div className="p-3 bg-rose-500/10 text-cyber-rose rounded-lg border border-cyber-rose/20">
              <AlertTriangle className="w-5 h-5 animate-pulse" />
            </div>
            <div>
              <span className="text-[10px] text-slate-500 font-mono uppercase tracking-wider block">Critical Assets</span>
              <span className="text-xl font-bold text-white">
                {assets.filter(a => a.criticality === 'Critical').length}
              </span>
            </div>
          </div>
          
          <div className="glass-panel p-4 rounded-xl flex items-center gap-4">
            <div className="p-3 bg-amber-500/10 text-cyber-amber rounded-lg border border-cyber-amber/20">
              <TrendingUp className="w-5 h-5" />
            </div>
            <div>
              <span className="text-[10px] text-slate-500 font-mono uppercase tracking-wider block">High Risk Index (&gt;60)</span>
              <span className="text-xl font-bold text-white">
                {assets.filter(a => a.riskScore > 60).length}
              </span>
            </div>
          </div>

          <div className="glass-panel p-4 rounded-xl flex items-center gap-4">
            <div className="p-3 bg-emerald-500/10 text-cyber-emerald rounded-lg border border-cyber-emerald/20">
              <Activity className="w-5 h-5" />
            </div>
            <div>
              <span className="text-[10px] text-slate-500 font-mono uppercase tracking-wider block">Active Malfunctions</span>
              <span className="text-xl font-bold text-white">
                {assets.reduce((acc, curr) => acc + curr.failures.filter(f => f.status === 'Open').length, 0)}
              </span>
            </div>
          </div>
        </div>

        {/* Filters and List */}
        <div className="glass-panel p-6 rounded-2xl border-slate-800/80">
          <div className="flex flex-col md:flex-row gap-4 justify-between items-start md:items-center mb-6">
            <div>
              <h3 className="text-base font-bold text-white tracking-wide">Asset Registry</h3>
              <p className="text-xs text-slate-400 mt-0.5">Explore physical asset nodes and operational intelligence</p>
            </div>

            {/* Filter Bar */}
            <div className="flex flex-wrap items-center gap-3 w-full md:w-auto">
              {/* Search */}
              <div className="relative flex-1 md:flex-none">
                <Search className="w-3.5 h-3.5 text-slate-500 absolute left-3 top-1/2 -translate-y-1/2" />
                <input
                  type="text"
                  value={searchQuery}
                  onChange={(e) => setSearchQuery(e.target.value)}
                  placeholder="Search by tag or name..."
                  className="w-full md:w-56 pl-9 pr-4 py-2 rounded-xl bg-slate-900/60 border border-slate-800 hover:border-slate-700/50 text-xs font-semibold text-white focus:outline-none focus:border-cyber-emerald"
                />
              </div>

              {/* Criticality */}
              <div className="relative">
                <Filter className="w-3.5 h-3.5 text-slate-500 absolute left-3 top-1/2 -translate-y-1/2" />
                <select
                  value={criticalityFilter}
                  onChange={(e) => setCriticalityFilter(e.target.value)}
                  className="pl-9 pr-8 py-2 rounded-xl bg-slate-900/60 border border-slate-800 hover:border-slate-700/50 text-xs font-semibold text-slate-300 focus:outline-none focus:border-cyber-emerald cursor-pointer appearance-none"
                >
                  <option value="All">All Criticalities</option>
                  <option value="Critical">Critical</option>
                  <option value="High">High</option>
                  <option value="Medium">Medium</option>
                  <option value="Low">Low</option>
                </select>
              </div>

              {/* Type */}
              <div className="relative">
                <Filter className="w-3.5 h-3.5 text-slate-500 absolute left-3 top-1/2 -translate-y-1/2" />
                <select
                  value={typeFilter}
                  onChange={(e) => setTypeFilter(e.target.value)}
                  className="pl-9 pr-8 py-2 rounded-xl bg-slate-900/60 border border-slate-800 hover:border-slate-700/50 text-xs font-semibold text-slate-300 focus:outline-none focus:border-cyber-emerald cursor-pointer appearance-none"
                >
                  <option value="All">All Types</option>
                  {assetTypes.map(t => (
                    <option key={t} value={t}>{t}</option>
                  ))}
                </select>
              </div>
            </div>
          </div>

          {/* Grid of cards */}
          <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-6">
            {filteredAssets.length > 0 ? (
              filteredAssets.map((asset) => {
                const openFailures = asset.failures.filter(f => f.status === 'Open').length;
                const linkedDocs = asset.documents.length;
                
                return (
                  <div 
                    key={asset.id}
                    className="p-5 bg-[#060918]/60 border border-slate-800/80 hover:border-slate-700/50 rounded-2xl flex flex-col justify-between hover:-translate-y-1 transition-all duration-300 group"
                  >
                    <div>
                      {/* Card Header */}
                      <div className="flex items-start justify-between mb-4">
                        <div className="flex items-center gap-2">
                          <span className="text-[10px] font-mono font-bold bg-slate-800 text-slate-300 border border-slate-700 px-2 py-0.5 rounded uppercase">
                            {asset.assetTag}
                          </span>
                          <span className={`text-[9px] font-mono font-bold uppercase border px-2 py-0.5 rounded-full ${
                            asset.criticality === 'Critical' ? 'bg-cyber-rose/10 text-cyber-rose border-cyber-rose/20' :
                            asset.criticality === 'High' ? 'bg-cyber-amber/10 text-cyber-amber border-cyber-amber/20' :
                            asset.criticality === 'Medium' ? 'bg-cyber-blue/10 text-cyber-blue border-cyber-blue/20' :
                            'bg-slate-800 text-slate-400 border-slate-700'
                          }`}>
                            {asset.criticality}
                          </span>
                        </div>
                        
                        {/* Risk Gauge */}
                        <div className="flex items-center gap-1.5 font-mono text-xs">
                          <span className="text-[10px] text-slate-500 font-semibold">Risk:</span>
                          <span className={`font-bold ${
                            asset.riskScore > 70 ? 'text-cyber-rose' :
                            asset.riskScore > 40 ? 'text-cyber-amber' : 'text-cyber-emerald'
                          }`}>{asset.riskScore}</span>
                        </div>
                      </div>

                      {/* Info */}
                      <h4 className="text-sm font-bold text-white tracking-wide truncate group-hover:text-cyber-emerald transition-colors">
                        {asset.assetName}
                      </h4>
                      <p className="text-xs text-slate-400 mt-1 truncate">{asset.location}</p>

                      {/* Stat Counters */}
                      <div className="grid grid-cols-2 gap-3 mt-6 border-t border-slate-800/60 pt-4 text-xs font-mono">
                        <div className="flex items-center gap-2">
                          <FileText className="w-3.5 h-3.5 text-slate-500" />
                          <div>
                            <span className="text-slate-500 text-[10px] block uppercase">Manuals/Docs</span>
                            <span className="text-slate-200 font-bold">{linkedDocs} linked</span>
                          </div>
                        </div>
                        <div className="flex items-center gap-2">
                          <Activity className="w-3.5 h-3.5 text-slate-500" />
                          <div>
                            <span className="text-slate-500 text-[10px] block uppercase">Open Incidents</span>
                            <span className={`font-bold ${openFailures > 0 ? 'text-cyber-rose' : 'text-slate-400'}`}>
                              {openFailures} failures
                            </span>
                          </div>
                        </div>
                      </div>
                    </div>

                    <Link 
                      href={`/assets/${asset.assetTag}`}
                      className="mt-6 w-full py-2 bg-slate-900/60 hover:bg-cyber-emerald/10 border border-slate-800 hover:border-cyber-emerald/30 text-slate-300 hover:text-cyber-emerald font-semibold rounded-xl text-xs transition-all flex items-center justify-center gap-1.5 group-hover:shadow-[0_0_15px_rgba(16,185,129,0.05)]"
                    >
                      View Profile <ArrowRight className="w-3 h-3" />
                    </Link>
                  </div>
                );
              })
            ) : (
              <div className="col-span-full py-12 text-center text-slate-500 font-mono text-xs">
                No plant equipment assets matched the search filters.
              </div>
            )}
          </div>
        </div>

      </div>
    </NavigationShell>
  );
}
