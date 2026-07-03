'use client';

import React, { useEffect, useState } from 'react';
import NavigationShell from '../../../components/NavigationShell';
import { useData } from '../../../context/DataContext';
import { apiFetch } from '../../../lib/api';
import { Asset, ComplianceGap, Document, FailureEvent } from '../../../lib/mockData';
import { 
  Wrench, 
  AlertTriangle, 
  FileText, 
  Activity, 
  Clock, 
  CheckCircle2, 
  ArrowLeft,
  ShieldAlert,
  Send,
  PlusCircle,
  MessageSquareCode,
  LayoutGrid
} from 'lucide-react';
import Link from 'next/link';

export default function AssetProfilePage({ params }: { params: { tag: string } }) {
  const { tag } = params;
  const { assets, documents, complianceGaps, addFailureEvent } = useData();
  const [remoteAsset, setRemoteAsset] = useState<Asset | null>(null);
  const [remoteDocuments, setRemoteDocuments] = useState<Document[]>([]);
  const [remoteGaps, setRemoteGaps] = useState<ComplianceGap[]>([]);
  const [remoteFailures, setRemoteFailures] = useState<FailureEvent[]>([]);
  const [remoteChecked, setRemoteChecked] = useState(false);

  useEffect(() => {
    let cancelled = false;

    const loadAssetProfile = async () => {
      try {
        const res = await apiFetch(`/api/assets/${encodeURIComponent(tag)}`);
        if (!res.ok) return;

        const data = await res.json();
        if (cancelled) return;

        const mappedFailures = (data.failures || []).map((failure: any): FailureEvent => ({
          id: failure.id,
          date: failure.createdAt?.split('T')[0] || new Date().toISOString().split('T')[0],
          description: failure.failureSummary,
          severity: 'High',
          status: 'Open',
          workOrder: 'RCA',
          maintenanceAction: (failure.recommendations || []).join(', ') || 'Review RCA recommendations',
        }));

        const mappedDocuments = (data.documents || []).map((doc: any): Document => ({
          id: doc.id,
          title: doc.title,
          fileType: 'DOC',
          documentType: doc.documentType || 'Document',
          status: doc.status || 'COMPLETED',
          ocrConfidence: 0,
          classificationConfidence: 0,
          createdAt: doc.createdAt,
          size: 'Stored',
        }));

        const mappedGaps = (data.complianceGaps || []).map((gap: any): ComplianceGap => ({
          id: gap.id,
          assetTag: data.assetTag,
          gapType: gap.gapType,
          description: gap.description,
          severity: normalizeSeverity(gap.severity),
          status: gap.status === 'CLOSED' || gap.status === 'Closed' ? 'Closed' : 'Open',
          createdAt: gap.createdAt,
        }));

        setRemoteFailures(mappedFailures);
        setRemoteDocuments(mappedDocuments);
        setRemoteGaps(mappedGaps);
        setRemoteAsset({
          id: data.id,
          assetTag: data.assetTag,
          assetName: data.assetName || data.assetTag,
          assetType: data.assetType || 'Asset',
          location: data.location || 'Plant',
          criticality: normalizeCriticality(data.criticality),
          riskScore: Number(data.riskScore || 50),
          failures: mappedFailures,
          documents: mappedDocuments.map((doc: Document) => doc.id),
          complianceGaps: mappedGaps.map((gap: ComplianceGap) => gap.id),
        });
      } finally {
        if (!cancelled) {
          setRemoteChecked(true);
        }
      }
    };

    loadAssetProfile().catch(() => {
      if (!cancelled) {
        setRemoteChecked(true);
      }
    });

    return () => {
      cancelled = true;
    };
  }, [tag]);
  
  // Find asset
  const asset = remoteAsset || assets.find(a => a.assetTag.toUpperCase() === tag.toUpperCase());

  // Incident Form state
  const [showLogForm, setShowLogForm] = useState(false);
  const [description, setDescription] = useState('');
  const [severity, setSeverity] = useState<'Low' | 'Medium' | 'High' | 'Critical'>('High');

  if (!asset && !remoteChecked) {
    return (
      <NavigationShell>
        <div className="flex flex-col items-center justify-center py-20 text-center">
          <Activity className="w-10 h-10 text-cyber-emerald animate-spin mb-4" />
          <p className="text-xs text-slate-400 font-mono">Loading asset intelligence...</p>
        </div>
      </NavigationShell>
    );
  }

  if (!asset) {
    return (
      <NavigationShell>
        <div className="flex flex-col items-center justify-center py-20 text-center">
          <AlertTriangle className="w-12 h-12 text-cyber-rose mb-4 animate-bounce" />
          <h2 className="text-xl font-bold text-white mb-2">Asset Not Found</h2>
          <p className="text-xs text-slate-400 font-mono mb-6">Equipment tag &ldquo;{tag}&rdquo; does not exist in the plant registry.</p>
          <Link 
            href="/assets" 
            className="px-4 py-2 bg-slate-800 border border-slate-700 hover:border-slate-600 rounded-xl text-xs font-semibold flex items-center gap-1.5 transition-all text-slate-300 hover:text-white"
          >
            <ArrowLeft className="w-3.5 h-3.5" /> Back to Assets
          </Link>
        </div>
      </NavigationShell>
    );
  }

  // Find linked documents
  const linkedDocuments = remoteDocuments.length > 0
    ? remoteDocuments
    : documents.filter(d => asset.documents.includes(d.id));

  // Find asset-specific compliance gaps
  const assetGaps = remoteGaps.length > 0
    ? remoteGaps
    : complianceGaps.filter(g => g.assetTag.toUpperCase() === asset.assetTag.toUpperCase());
  const inspectionDocs = linkedDocuments.filter(doc => doc.documentType.toLowerCase().includes('inspection'));
  const maintenanceDocs = linkedDocuments.filter(doc => doc.documentType.toLowerCase().includes('maintenance') || doc.title.toLowerCase().includes('work order'));
  const openRisks = [
    ...asset.failures.filter(failure => failure.status === 'Open').map(failure => failure.description),
    ...assetGaps.filter(gap => gap.status === 'Open').map(gap => gap.gapType),
  ];
  const recommendedActions = [
    ...asset.failures.filter(failure => failure.status === 'Open').map(failure => failure.maintenanceAction),
    ...assetGaps.filter(gap => gap.status === 'Open').map(gap => `Provide evidence or close: ${gap.gapType}`),
  ].slice(0, 4);

  const handleLogFailure = (e: React.FormEvent) => {
    e.preventDefault();
    if (description.trim()) {
      addFailureEvent(asset.assetTag, description, severity);
      setDescription('');
      setShowLogForm(false);
    }
  };

  return (
    <NavigationShell>
      <div className="space-y-6">
        
        {/* Back Link & Quick Navigation */}
        <div className="flex flex-wrap items-center justify-between gap-4">
          <Link 
            href="/assets" 
            className="text-xs text-slate-400 hover:text-cyber-emerald flex items-center gap-1.5 transition-colors font-semibold"
          >
            <ArrowLeft className="w-3.5 h-3.5" /> Back to Asset Catalog
          </Link>
          
          <div className="flex gap-2">
            <Link
              href={`/copilot?tag=${asset.assetTag}`}
              className="px-3 py-1.5 bg-slate-900 border border-slate-800 hover:border-cyber-emerald/30 text-slate-300 hover:text-cyber-emerald font-semibold rounded-xl text-xs flex items-center gap-1.5 transition-all"
            >
              <MessageSquareCode className="w-3.5 h-3.5" /> Copilot Query
            </Link>
            <Link
              href={`/rca?tag=${asset.assetTag}`}
              className="px-3 py-1.5 bg-slate-900 border border-slate-800 hover:border-cyber-amber/30 text-slate-300 hover:text-cyber-amber font-semibold rounded-xl text-xs flex items-center gap-1.5 transition-all"
            >
              <Activity className="w-3.5 h-3.5" /> RCA Assistant
            </Link>
          </div>
        </div>

        {/* Profile Card Header */}
        <div className="glass-panel p-6 rounded-2xl border-slate-800/80 grid grid-cols-1 lg:grid-cols-3 gap-6 items-center">
          <div className="lg:col-span-2 space-y-2">
            <div className="flex items-center gap-2">
              <span className="text-xs font-mono font-bold bg-slate-800 text-slate-300 border border-slate-700 px-2 py-0.5 rounded uppercase">
                {asset.assetTag}
              </span>
              <span className={`text-[10px] font-mono font-bold uppercase border px-2.5 py-0.5 rounded-full ${
                asset.criticality === 'Critical' ? 'bg-cyber-rose/10 text-cyber-rose border-cyber-rose/20 shadow-[0_0_15px_rgba(244,63,94,0.05)]' :
                asset.criticality === 'High' ? 'bg-cyber-amber/10 text-cyber-amber border-cyber-amber/20' :
                'bg-slate-800 text-slate-400 border-slate-700'
              }`}>
                {asset.criticality} Criticality
              </span>
            </div>
            <h2 className="text-xl font-bold text-white tracking-wide">{asset.assetName}</h2>
            <p className="text-xs text-slate-400 font-mono">Location: <span className="text-slate-300">{asset.location}</span> • Type: <span className="text-slate-300">{asset.assetType}</span></p>
          </div>

          <div className="lg:col-span-1 bg-[#060918]/60 border border-slate-800 p-4 rounded-xl flex items-center justify-between gap-4">
            <div>
              <span className="text-[10px] text-slate-500 font-mono uppercase tracking-wider block">Risk Severity Index</span>
              <span className="text-2xl font-black text-white">{asset.riskScore} <span className="text-slate-500 text-xs font-normal">/ 100</span></span>
            </div>
            
            {/* Risk Gauge Bar */}
            <div className="w-24 h-2 bg-slate-800 rounded-full overflow-hidden relative border border-slate-700">
              <div 
                className={`h-full rounded-full ${
                  asset.riskScore > 70 ? 'bg-cyber-rose' :
                  asset.riskScore > 40 ? 'bg-cyber-amber' : 'bg-cyber-emerald'
                }`}
                style={{ width: `${asset.riskScore}%` }}
              />
            </div>
          </div>
        </div>

        <div className="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-4 gap-4">
          <ProfileMetric
            icon={<FileText className="w-4 h-4" />}
            label="Linked Evidence"
            value={`${linkedDocuments.length} docs`}
            detail={`${maintenanceDocs.length} maintenance / ${inspectionDocs.length} inspection`}
          />
          <ProfileMetric
            icon={<Wrench className="w-4 h-4" />}
            label="Maintenance History"
            value={`${asset.failures.length} events`}
            detail={asset.failures.some(failure => failure.status === 'Open') ? 'Open event requires RCA' : 'No open event'}
          />
          <ProfileMetric
            icon={<CheckCircle2 className="w-4 h-4" />}
            label="Inspection Status"
            value={inspectionDocs.length > 0 ? 'Evidence linked' : 'Evidence missing'}
            detail={inspectionDocs[0]?.title || 'No inspection document linked'}
          />
          <ProfileMetric
            icon={<ShieldAlert className="w-4 h-4" />}
            label="Open Risks"
            value={`${openRisks.length} active`}
            detail={openRisks[0] || 'No active risk item'}
          />
        </div>

        {/* Main Grid: Left Timeline / Right Specs */}
        <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
          
          {/* Left Column: Failure Timeline */}
          <div className="lg:col-span-2 space-y-6">
            
            {/* Timeline */}
            <div className="glass-panel p-6 rounded-2xl border-slate-800/80">
              <div className="flex items-center justify-between mb-6">
                <div>
                  <h3 className="text-base font-bold text-white tracking-wide">Failure History Timeline</h3>
                  <p className="text-xs text-slate-400 mt-0.5">Chronological record of failures, malfunctions, and diagnostic actions</p>
                </div>
                
                <button
                  onClick={() => setShowLogForm(!showLogForm)}
                  className="px-2.5 py-1.5 bg-cyber-emerald hover:bg-emerald-600 active:bg-emerald-700 text-slate-950 font-bold rounded-xl text-xs flex items-center gap-1 transition-all"
                >
                  <PlusCircle className="w-3.5 h-3.5" /> Log Failure
                </button>
              </div>

              {/* Log Failure Mock Form */}
              {showLogForm && (
                <div className="p-4 bg-[#060918]/80 border border-emerald-500/20 rounded-xl mb-6 space-y-4">
                  <h4 className="text-xs font-bold text-cyber-emerald uppercase tracking-wider">Report Operational Event</h4>
                  
                  <form onSubmit={handleLogFailure} className="space-y-3">
                    <div>
                      <label className="block text-[10px] font-bold text-slate-500 uppercase tracking-wider mb-1">Failure Description</label>
                      <input 
                        type="text" 
                        value={description}
                        onChange={(e) => setDescription(e.target.value)}
                        placeholder="e.g. Steam seal blowout, casing crack, vibration spikes..."
                        className="w-full px-3 py-2 rounded-lg glass-input text-xs font-medium"
                      />
                    </div>
                    
                    <div>
                      <label className="block text-[10px] font-bold text-slate-500 uppercase tracking-wider mb-1">Severity</label>
                      <select
                        value={severity}
                        onChange={(e) => setSeverity(e.target.value as any)}
                        className="w-full px-3 py-2 rounded-lg glass-input text-xs font-medium"
                      >
                        <option value="Low">Low</option>
                        <option value="Medium">Medium</option>
                        <option value="High">High</option>
                        <option value="Critical">Critical</option>
                      </select>
                    </div>

                    <div className="flex gap-2 justify-end pt-1">
                      <button
                        type="button"
                        onClick={() => setShowLogForm(false)}
                        className="px-3 py-1.5 border border-slate-800 hover:bg-slate-800/40 text-slate-400 font-semibold rounded-lg text-xs transition-all"
                      >
                        Cancel
                      </button>
                      <button
                        type="submit"
                        className="px-3 py-1.5 bg-cyber-emerald hover:bg-emerald-600 text-slate-950 font-bold rounded-lg text-xs transition-all"
                      >
                        Log Event
                      </button>
                    </div>
                  </form>
                </div>
              )}

              {/* Chronological Timeline */}
              <div className="space-y-6 relative before:absolute before:left-3 before:top-2 before:bottom-2 before:w-px before:bg-slate-800">
                {asset.failures.length > 0 ? (
                  asset.failures.map((fail) => {
                    const isOpen = fail.status === 'Open';
                    return (
                      <div key={fail.id} className="relative pl-8 flex gap-4 items-start group">
                        
                        {/* Dot Marker */}
                        <span className={`absolute left-1.5 w-3.5 h-3.5 rounded-full border-4 border-[#020617] -translate-x-1/2 z-10 ${
                          isOpen 
                            ? 'bg-cyber-rose shadow-[0_0_10px_rgba(244,63,94,0.6)] animate-pulse' 
                            : 'bg-cyber-emerald'
                        }`} />

                        <div className="flex-1 p-4 bg-[#060918]/60 border border-slate-800/80 group-hover:border-slate-700/50 rounded-xl transition-all">
                          {/* Fail header */}
                          <div className="flex items-center justify-between gap-4 mb-2">
                            <span className="text-[10px] text-slate-500 font-mono">{fail.date}</span>
                            <div className="flex items-center gap-1.5">
                              <span className={`text-[9px] font-mono font-bold uppercase border px-1.5 py-0.5 rounded ${
                                fail.severity === 'Critical' ? 'bg-cyber-rose/10 text-cyber-rose border-cyber-rose/20' :
                                fail.severity === 'High' ? 'bg-cyber-amber/10 text-cyber-amber border-cyber-amber/20' :
                                'bg-slate-800 text-slate-400 border-slate-700'
                              }`}>
                                {fail.severity}
                              </span>
                              
                              <span className={`text-[9px] font-mono font-bold uppercase border px-1.5 py-0.5 rounded ${
                                isOpen ? 'bg-cyber-rose/10 text-cyber-rose border-cyber-rose/20' : 'bg-cyber-emerald/10 text-cyber-emerald border-cyber-emerald/20'
                              }`}>
                                {fail.status}
                              </span>
                            </div>
                          </div>

                          <h4 className="text-xs font-bold text-slate-200 tracking-wide mb-1.5">{fail.description}</h4>
                          
                          {/* WO and Actions */}
                          <div className="mt-3 pt-3 border-t border-slate-800/60 grid grid-cols-1 sm:grid-cols-2 gap-3 text-[10px] font-mono">
                            <div>
                              <span className="text-slate-500 block">Work Order</span>
                              <span className="text-slate-300 font-semibold">{fail.workOrder}</span>
                            </div>
                            <div>
                              <span className="text-slate-500 block">Resolution Action</span>
                              <span className="text-slate-300">{fail.maintenanceAction}</span>
                            </div>
                          </div>
                        </div>
                      </div>
                    );
                  })
                ) : (
                  <div className="py-8 text-center text-slate-500 font-mono text-xs pl-8">
                    No recorded failure history.
                  </div>
                )}
              </div>
            </div>
          </div>

          {/* Right Column: Gaps & Manual References */}
          <div className="lg:col-span-1 space-y-6">
            
            {/* Active Gaps */}
            <div className="glass-panel p-6 rounded-2xl border-slate-800/80">
              <h3 className="text-xs font-bold text-white tracking-wider uppercase mb-4 flex items-center gap-1.5">
                <ShieldAlert className="w-4 h-4 text-cyber-rose" /> Compliance Gaps
              </h3>

              <div className="space-y-3">
                {assetGaps.length > 0 ? (
                  assetGaps.map(gap => (
                    <div 
                      key={gap.id}
                      className="p-3 bg-[#060918]/60 border border-slate-800 rounded-xl text-xs space-y-1.5"
                    >
                      <div className="flex items-center justify-between">
                        <span className="font-bold text-slate-200">{gap.gapType}</span>
                        <span className={`text-[8px] font-mono uppercase px-1.5 py-0.5 rounded border ${
                          gap.severity === 'Critical' ? 'bg-cyber-rose/10 text-cyber-rose border-cyber-rose/20' :
                          gap.severity === 'High' ? 'bg-cyber-amber/10 text-cyber-amber border-cyber-amber/20' :
                          'bg-slate-800 text-slate-400 border-slate-700'
                        }`}>
                          {gap.severity}
                        </span>
                      </div>
                      <p className="text-[10px] text-slate-400">{gap.description}</p>
                    </div>
                  ))
                ) : (
                  <div className="py-6 text-center text-slate-500 font-mono text-xs border border-dashed border-slate-800 rounded-xl">
                    No active compliance gaps.
                  </div>
                )}
              </div>
            </div>

            <div className="glass-panel p-6 rounded-2xl border-slate-800/80">
              <h3 className="text-xs font-bold text-white tracking-wider uppercase mb-4 flex items-center gap-1.5">
                <CheckCircle2 className="w-4 h-4 text-cyber-emerald" /> Recommended Actions
              </h3>

              <div className="space-y-2">
                {recommendedActions.length > 0 ? recommendedActions.map((action, index) => (
                  <div key={`${action}-${index}`} className="p-3 bg-emerald-500/5 border border-emerald-500/10 rounded-xl text-xs text-slate-300 flex gap-2">
                    <span className="text-cyber-emerald font-mono font-bold">{index + 1}</span>
                    <span>{action}</span>
                  </div>
                )) : (
                  <div className="py-6 text-center text-slate-500 font-mono text-xs border border-dashed border-slate-800 rounded-xl">
                    No immediate recommendations.
                  </div>
                )}
              </div>

              <Link
                href="/graph"
                className="mt-4 w-full py-2 bg-slate-900/60 hover:bg-cyber-emerald/10 border border-slate-800 hover:border-cyber-emerald/30 text-slate-300 hover:text-cyber-emerald font-semibold rounded-xl text-xs transition-all flex items-center justify-center gap-1.5"
              >
                <LayoutGrid className="w-3.5 h-3.5" /> View Graph Relationships
              </Link>
            </div>

            {/* Manual & Reference Documents */}
            <div className="glass-panel p-6 rounded-2xl border-slate-800/80">
              <h3 className="text-xs font-bold text-white tracking-wider uppercase mb-4 flex items-center gap-1.5">
                <FileText className="w-4 h-4 text-cyber-blue" /> Knowledge References
              </h3>

              <div className="space-y-3">
                {linkedDocuments.length > 0 ? (
                  linkedDocuments.map(doc => (
                    <div 
                      key={doc.id}
                      className="p-3 bg-[#060918]/60 border border-slate-800/80 hover:border-slate-700/50 rounded-xl flex items-center justify-between gap-3 text-xs"
                    >
                      <div className="flex items-center gap-2 min-w-0">
                        <FileText className="w-3.5 h-3.5 text-cyber-blue shrink-0" />
                        <span className="font-semibold text-slate-300 truncate" title={doc.title}>{doc.title}</span>
                      </div>
                      <span className="text-[9px] font-mono text-slate-500 shrink-0">{doc.documentType}</span>
                    </div>
                  ))
                ) : (
                  <div className="py-6 text-center text-slate-500 font-mono text-xs border border-dashed border-slate-800 rounded-xl">
                    No documents linked to this asset.
                  </div>
                )}
              </div>
            </div>

          </div>

        </div>

      </div>
    </NavigationShell>
  );
}

function normalizeCriticality(value: string | undefined): Asset['criticality'] {
  const normalized = (value || 'Medium').toLowerCase();
  if (normalized === 'critical') return 'Critical';
  if (normalized === 'high') return 'High';
  if (normalized === 'low') return 'Low';
  return 'Medium';
}

function normalizeSeverity(value: string | undefined): ComplianceGap['severity'] {
  const normalized = (value || 'Medium').toLowerCase();
  if (normalized === 'critical') return 'Critical';
  if (normalized === 'high') return 'High';
  if (normalized === 'low') return 'Low';
  return 'Medium';
}

function ProfileMetric({ icon, label, value, detail }: { icon: React.ReactNode; label: string; value: string; detail: string }) {
  return (
    <div className="glass-panel p-4 rounded-xl border-slate-800/80 min-w-0">
      <div className="flex items-center gap-3">
        <div className="p-2 bg-emerald-500/10 text-cyber-emerald rounded-lg border border-emerald-500/20 shrink-0">
          {icon}
        </div>
        <div className="min-w-0">
          <span className="text-[10px] text-slate-500 font-mono uppercase tracking-wider block">{label}</span>
          <span className="text-sm font-bold text-white block truncate">{value}</span>
        </div>
      </div>
      <p className="text-[10px] text-slate-500 mt-3 truncate" title={detail}>{detail}</p>
    </div>
  );
}
