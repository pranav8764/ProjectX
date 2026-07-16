'use client';

import React, { Suspense, useState, useEffect } from 'react';
import NavigationShell from '../../components/NavigationShell';
import { useData } from '../../context/DataContext';
import { RcaReport } from '../../lib/mockData';
import { apiFetch, getStoredSession, PlantBrainSession, readApiError } from '../../lib/api';
import { canRunAction, getDeniedMessage } from '../../lib/permissions';
import { useSearchParams } from 'next/navigation';
import { 
  Activity, 
  Wrench, 
  ArrowRight, 
  Sparkles, 
  CheckCircle2, 
  AlertTriangle, 
  Cpu, 
  FileDown,
  Layers,
  ArrowDown
} from 'lucide-react';
import Link from 'next/link';

type ApiRcaResponse = Partial<RcaReport> & {
  failureSummary?: string;
};

function normalizeRcaReport(data: ApiRcaResponse, assetTag: string, description: string): RcaReport {
  const probableCauses = Array.isArray(data.probableCauses) ? data.probableCauses : [];
  const recommendations = Array.isArray(data.recommendations) ? data.recommendations : [];
  const summary = data.summary || data.failureSummary || `RCA report generated for ${assetTag}: ${description}`;
  const fiveWhys = Array.isArray(data.fiveWhys) && data.fiveWhys.length > 0
    ? data.fiveWhys
    : probableCauses.slice(0, 5).map((cause, index) => `Why ${index + 1}? - ${cause}`);

  return {
    summary,
    fiveWhys,
    probableCauses,
    recommendations,
    confidence: typeof data.confidence === 'number' ? data.confidence : 0,
    citations: Array.isArray(data.citations) ? data.citations : [],
    missingData: Array.isArray(data.missingData) ? data.missingData : [],
  };
}

function RcaAssistantPageContent() {
  const searchParams = useSearchParams();
  const { assets } = useData();
  const initialTag = searchParams.get('tag') || '';

  const [assetTag, setAssetTag] = useState(initialTag);
  const [description, setDescription] = useState('');
  const [loading, setLoading] = useState(false);
  const [report, setReport] = useState<RcaReport | null>(null);
  const [session, setSession] = useState<PlantBrainSession | null>(null);
  const [actionError, setActionError] = useState('');
  const canGenerateRca = canRunAction(session?.role, 'generate_rca');

  useEffect(() => {
    setSession(getStoredSession());
  }, []);

  useEffect(() => {
    if (initialTag) {
      setAssetTag(initialTag);
      // Pre-fill default failure if matching P-101
      if (initialTag === 'P-101') {
        setDescription('Repeated seal leakage and pressure drop across intake chamber');
      } else {
        setDescription('Unidentified operational event / component wear');
      }
    }
  }, [initialTag]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!assetTag || !description) return;

    if (!canGenerateRca) {
      setActionError(getDeniedMessage(session?.role, 'generate_rca'));
      return;
    }

    setLoading(true);
    setReport(null);
    setActionError('');

    // Call API / Fallback Mock
    try {
      const payload = {
        assetTag,
        failureDescription: description
      };

      // Try calling Go API
      const res = await apiFetch('/api/rca/generate', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload)
      });

      if (res.status === 403) {
        setActionError(await readApiError(res, 'Your role cannot generate RCA reports.'));
        setLoading(false);
        return;
      }

      if (!res.ok) throw new Error('RCA service offline');

      const data = await res.json();
      setReport(normalizeRcaReport(data, assetTag, description));

    } catch (err) {
      // No fabricated RCA: showing a canned root-cause analysis with fake confidence
      // would be an unsupported claim. Surface the failure instead.
      const message = err instanceof Error ? err.message : 'The RCA service is unavailable.';
      setActionError(`Unable to generate an RCA right now. ${message} Please retry once the AI service is available.`);
      setLoading(false);
      return;
    }
    setLoading(false);
  };

  const handleExport = () => {
    if (!report) return;
    const content = [
      'ROOT CAUSE ANALYSIS REPORT',
      `Asset Tag: ${assetTag}`,
      `Event: ${description}`,
      `Generated: ${new Date().toISOString()}`,
      `Confidence: ${((report.confidence || 0) * 100).toFixed(0)}%`,
      '',
      'SUMMARY',
      report.summary,
      '',
      '5-WHYS CHAIN',
      ...(report.fiveWhys || []).map((why, index) => `${index + 1}. ${why}`),
      '',
      'PROBABLE CAUSES',
      ...(report.probableCauses || []).map((cause, index) => `${index + 1}. ${cause}`),
      '',
      'RECOMMENDATIONS',
      ...(report.recommendations || []).map((rec, index) => `${index + 1}. ${rec}`),
      '',
      'SUPPORTING EVIDENCE',
      ...((report.citations || []).length
        ? (report.citations || []).map(citation => `- ${citation.documentTitle}${citation.page ? ` p.${citation.page}` : ''}${citation.snippet ? `: ${citation.snippet}` : ''}`)
        : ['- No citation returned by the RCA service.']),
      '',
      'MISSING DATA',
      ...(report.missingData?.length ? report.missingData.map(item => `- ${item}`) : ['- No missing data flagged.']),
    ].join('\n');
    const blob = new Blob([content], { type: 'text/plain' });
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = `RCA_Report_${assetTag}_${new Date().toISOString().split('T')[0]}.txt`;
    link.click();
  };

  return (
    <NavigationShell>
      <div className="grid grid-cols-1 xl:grid-cols-3 gap-8">
        
        {/* Left Panel: Form */}
        <div className="xl:col-span-1 space-y-6">
          <div className="glass-panel p-6 rounded-2xl border-slate-800/80">
            <div className="flex items-center gap-2 mb-4">
              <Sparkles className="w-5 h-5 text-cyber-amber" />
              <h3 className="text-base font-bold text-white tracking-wide">Generate RCA Report</h3>
            </div>

            <form onSubmit={handleSubmit} className="space-y-4">
              {actionError && (
                <div className="p-3 bg-cyber-amber/5 border border-cyber-amber/20 text-amber-200 rounded-xl text-xs flex gap-2">
                  <AlertTriangle className="w-4 h-4 shrink-0 text-cyber-amber" />
                  <span>{actionError}</span>
                </div>
              )}

              <div>
                <label className="block text-[10px] font-bold text-slate-500 uppercase tracking-wider mb-1.5">Target Equipment Tag</label>
                <select
                  value={assetTag}
                  onChange={(e) => setAssetTag(e.target.value)}
                  className="w-full px-4 py-3 rounded-xl glass-input text-xs font-medium"
                  required
                >
                  <option value="">Select Asset Tag</option>
                  {assets.map(a => (
                    <option key={a.id} value={a.assetTag}>{a.assetTag} - {a.assetName}</option>
                  ))}
                </select>
              </div>

              <div>
                <label className="block text-[10px] font-bold text-slate-500 uppercase tracking-wider mb-1.5">Failure Incident Description</label>
                <textarea
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                  placeholder="Describe the failure symptoms, temperature spikes, fluid leaks, or abnormal vibrations..."
                  rows={4}
                  className="w-full px-4 py-3 rounded-xl glass-input text-xs font-medium resize-none"
                  required
                />
              </div>

              <button
                type="submit"
                disabled={loading || !assetTag || !description || !canGenerateRca}
                title={!canGenerateRca ? getDeniedMessage(session?.role, 'generate_rca') : undefined}
                className="w-full py-3 bg-cyber-amber hover:bg-amber-600 active:bg-amber-700 text-slate-950 font-bold rounded-xl text-xs transition-all hover:shadow-[0_0_15px_rgba(245,158,11,0.2)] disabled:opacity-50 disabled:cursor-not-allowed"
              >
                {loading ? 'Synthesizing RCA...' : 'Synthesize Root Causes'}
              </button>
            </form>
          </div>

          {/* Quick tips panel */}
          <div className="glass-panel p-6 rounded-2xl border-slate-800/80 text-xs">
            <h4 className="text-[10px] font-bold text-white uppercase tracking-wider mb-3">RCA Methodology</h4>
            <p className="text-slate-400 leading-relaxed mb-3">
              Our agent evaluates the complete asset history, work orders, incident reports, and OEM specification documents to assemble a 5-Whys diagnostic chain.
            </p>
            <div className="flex items-center gap-1.5 text-cyber-amber font-mono font-semibold">
              <Activity className="w-4 h-4" /> 5-Whys Loop Active
            </div>
          </div>
        </div>

        {/* Right Panel: RCA Results */}
        <div className="xl:col-span-2 space-y-6">
          {loading ? (
            <div className="glass-panel p-12 rounded-2xl border-slate-800/80 flex flex-col items-center justify-center text-center gap-4 h-96">
              <Cpu className="w-12 h-12 text-cyber-amber animate-spin" />
              <div>
                <h4 className="text-sm font-bold text-white font-mono">Assembling Failure History Timeline...</h4>
                <p className="text-xs text-slate-500 font-mono mt-1">Cross-referencing OEM manuals, drawings, and work orders</p>
              </div>
            </div>
          ) : report ? (
            <div className="glass-panel p-6 rounded-2xl border-slate-800/80 space-y-6">
              
              {/* Header metrics */}
              <div className="flex flex-col sm:flex-row justify-between items-start sm:items-center border-b border-slate-800 pb-4 gap-4">
                <div>
                  <div className="flex items-center gap-2">
                    <span className="text-xs font-mono font-bold bg-slate-800 text-slate-300 border border-slate-700 px-2 py-0.5 rounded uppercase">
                      RCA FOR {assetTag}
                    </span>
                    <span className="text-[10px] bg-cyber-amber/10 text-cyber-amber border border-cyber-amber/20 px-2.5 py-0.5 rounded-full font-mono font-bold">
                      Confidence: {((report.confidence || 0) * 100).toFixed(0)}%
                    </span>
                  </div>
                  <h3 className="text-base font-bold text-white tracking-wide mt-2">Diagnostic Summary</h3>
                </div>

                <button 
                  onClick={handleExport}
                  className="px-3 py-2 bg-slate-800 hover:bg-slate-700 border border-slate-700 hover:border-slate-600 text-slate-300 hover:text-white rounded-xl text-xs font-semibold flex items-center gap-1.5 transition-all"
                >
                  <FileDown className="w-3.5 h-3.5" /> Export Report
                </button>
              </div>

              {/* Problem summary content */}
              <div className="p-4 bg-[#060918]/60 border border-slate-800 rounded-xl">
                <span className="text-[10px] text-slate-500 font-mono uppercase tracking-wider block mb-1">Incident Profile</span>
                <p className="text-xs text-slate-300 leading-relaxed font-semibold">{report.summary}</p>
              </div>

              {/* 5-Whys Diagram section */}
              <div>
                <h4 className="text-[10px] font-bold text-slate-500 uppercase tracking-widest mb-4">Interactive 5-Whys Chain</h4>
                <div className="flex flex-col items-center gap-4">
                  {(report.fiveWhys || []).map((why, index) => {
                    const parts = why.split('? - ');
                    const question = parts[0] + '?';
                    const answer = parts[1] || '';
                    return (
                      <React.Fragment key={index}>
                        <div className="w-full p-4 bg-[#060918]/80 border border-slate-800/80 hover:border-slate-700/50 rounded-xl text-xs flex gap-3 items-start transition-all">
                          <span className="w-7 h-7 bg-amber-500/10 text-cyber-amber border border-amber-500/20 rounded-lg flex items-center justify-center shrink-0 font-bold font-mono">
                            {index + 1}
                          </span>
                          <div>
                            <span className="font-bold text-slate-400 block font-mono text-[10px] uppercase">Why {index + 1}</span>
                            <span className="text-slate-200 block mt-0.5 font-semibold">{question}</span>
                            <span className="text-cyber-amber block mt-1 leading-relaxed">{answer}</span>
                          </div>
                        </div>
                        {index < (report.fiveWhys || []).length - 1 && (
                          <ArrowDown className="w-4 h-4 text-slate-700 animate-pulse" />
                        )}
                      </React.Fragment>
                    );
                  })}
                </div>
              </div>

              {/* Probable causes and recommendations */}
              <div className="grid grid-cols-1 md:grid-cols-2 gap-6 pt-4 border-t border-slate-800">
                
                {/* Causes */}
                <div className="space-y-3">
                  <h4 className="text-[10px] font-bold text-slate-500 uppercase tracking-widest">Contributing Factors</h4>
                  <div className="space-y-2">
                    {(report.probableCauses || []).map((cause, idx) => (
                      <div key={idx} className="p-3 bg-[#060918]/50 border border-slate-800 rounded-xl text-xs flex items-start gap-2.5 text-slate-300">
                        <AlertTriangle className="w-4 h-4 text-cyber-rose shrink-0 mt-0.5" />
                        <span>{cause}</span>
                      </div>
                    ))}
                  </div>
                </div>

                {/* Recommendations */}
                <div className="space-y-3">
                  <h4 className="text-[10px] font-bold text-slate-500 uppercase tracking-widest">Corrective Recommendations</h4>
                  <div className="space-y-2">
                    {(report.recommendations || []).map((rec, idx) => (
                      <div key={idx} className="p-3 bg-emerald-500/5 border border-emerald-500/10 rounded-xl text-xs flex items-start gap-2.5 text-slate-300">
                        <CheckCircle2 className="w-4 h-4 text-cyber-emerald shrink-0 mt-0.5" />
                        <span>{rec}</span>
                      </div>
                    ))}
                  </div>
                </div>

              </div>

              <div className="grid grid-cols-1 md:grid-cols-2 gap-6 pt-4 border-t border-slate-800">
                <div className="space-y-3">
                  <h4 className="text-[10px] font-bold text-slate-500 uppercase tracking-widest">Supporting Evidence</h4>
                  {(report.citations || []).length > 0 ? (
                    (report.citations || []).map((citation, idx) => (
                      <div key={idx} className="p-3 bg-[#060918]/50 border border-slate-800 rounded-xl text-xs text-slate-300">
                        <span className="font-bold text-cyber-blue block">{citation.documentTitle}{citation.page ? ` p.${citation.page}` : ''}</span>
                        {citation.snippet && <span className="text-[11px] text-slate-400 mt-1 block">{citation.snippet}</span>}
                      </div>
                    ))
                  ) : (
                    <div className="p-3 bg-cyber-amber/5 border border-cyber-amber/20 rounded-xl text-xs text-amber-200 flex gap-2">
                      <AlertTriangle className="w-4 h-4 shrink-0 text-cyber-amber" />
                      <span>No citation returned by the RCA service.</span>
                    </div>
                  )}
                </div>

                <div className="space-y-3">
                  <h4 className="text-[10px] font-bold text-slate-500 uppercase tracking-widest">Missing Data</h4>
                  {(report.missingData || []).length > 0 ? (
                    report.missingData?.map((item, idx) => (
                      <div key={idx} className="p-3 bg-cyber-rose/5 border border-cyber-rose/10 rounded-xl text-xs flex items-start gap-2.5 text-slate-300">
                        <AlertTriangle className="w-4 h-4 text-cyber-rose shrink-0 mt-0.5" />
                        <span>{item}</span>
                      </div>
                    ))
                  ) : (
                    <div className="p-3 bg-[#060918]/50 border border-slate-800 rounded-xl text-xs text-slate-400">
                      No missing data flagged.
                    </div>
                  )}
                </div>
              </div>

            </div>
          ) : (
            <div className="glass-panel p-12 rounded-2xl border-slate-800/80 flex flex-col items-center justify-center text-center gap-3 h-96">
              <Activity className="w-10 h-10 text-slate-600 animate-pulse-glow" />
              <h4 className="text-sm font-bold text-slate-400">RCA Assistant Standby</h4>
              <p className="text-xs text-slate-500 font-mono max-w-sm mt-1">Select an asset tag and specify the event description to synthesize the root causes.</p>
            </div>
          )}
        </div>

      </div>
    </NavigationShell>
  );
}

export default function RcaAssistantPage() {
  return (
    <Suspense fallback={null}>
      <RcaAssistantPageContent />
    </Suspense>
  );
}
