'use client';

import React, { useEffect, useState } from 'react';
import Link from 'next/link';
import NavigationShell from '../../components/NavigationShell';
import { useData } from '../../context/DataContext';
import { apiFetch, appendPlantQuery, getStoredSession, PlantBrainSession, readApiError } from '../../lib/api';
import { canRunAction, getDeniedMessage, getRoleLabel } from '../../lib/permissions';
import {
  ShieldAlert,
  Award,
  CheckCircle2,
  AlertTriangle,
  Check,
  Upload,
  Calendar,
  Layers,
  FileCheck,
  TrendingUp,
  XCircle,
  FileText,
  FileDown,
  Lock,
  RefreshCw
} from 'lucide-react';

export default function ComplianceAuditPage() {
  const { complianceGaps, certificates, resolveGap } = useData();
  const [activeTab, setActiveTab] = useState<'gaps' | 'certificates'>('gaps');
  const [severityFilter, setSeverityFilter] = useState('All');
  const [statusFilter, setStatusFilter] = useState('Open');
  const [session, setSession] = useState<PlantBrainSession | null>(null);
  const [sessionLoaded, setSessionLoaded] = useState(false);
  const [scanBusy, setScanBusy] = useState(false);
  const [actionMessage, setActionMessage] = useState('');

  useEffect(() => {
    setSession(getStoredSession());
    setSessionLoaded(true);
  }, []);

  const canViewCompliance = canRunAction(session?.role, 'view_compliance');
  const canScanCompliance = canRunAction(session?.role, 'run_compliance_scan');
  const canResolveGap = canRunAction(session?.role, 'resolve_compliance_gap');
  const canExportReports = canRunAction(session?.role, 'export_reports');

  // Counts
  const openGapsCount = complianceGaps.filter(g => g.status === 'Open').length;
  const expiredCertsCount = certificates.filter(c => c.status === 'Expired' || c.status === 'Overdue').length;
  const filteredGaps = complianceGaps.filter(gap => {
    const matchesSeverity = severityFilter === 'All' || gap.severity === severityFilter;
    const matchesStatus = statusFilter === 'All' || gap.status === statusFilter;
    return matchesSeverity && matchesStatus;
  });

  const handleRunScan = async () => {
    setActionMessage('');

    if (!canScanCompliance) {
      setActionMessage(getDeniedMessage(session?.role, 'run_compliance_scan'));
      return;
    }

    setScanBusy(true);
    try {
      const res = await apiFetch(appendPlantQuery('/api/compliance/scan'), { method: 'POST' });
      if (!res.ok) {
        throw new Error(await readApiError(res, 'Unable to run compliance scan'));
      }
      setActionMessage('Compliance scan completed. Refresh gap data from the API to inspect newly detected evidence conflicts.');
    } catch (error) {
      setActionMessage(error instanceof Error ? error.message : 'Unable to run compliance scan');
    } finally {
      setScanBusy(false);
    }
  };

  const handleExport = () => {
    setActionMessage('');
    if (!canExportReports) {
      setActionMessage(getDeniedMessage(session?.role, 'export_reports'));
      return;
    }
    exportComplianceCsv(activeTab, filteredGaps, certificates);
  };

  if (!sessionLoaded) {
    return (
      <NavigationShell>
        <div className="py-20 text-center text-slate-500 font-mono text-xs">Checking compliance access...</div>
      </NavigationShell>
    );
  }

  if (!canViewCompliance) {
    return (
      <NavigationShell>
        <div className="glass-panel p-8 rounded-2xl border-slate-800/80 text-center max-w-2xl mx-auto">
          <Lock className="w-10 h-10 text-cyber-amber mx-auto mb-4" />
          <h2 className="text-lg font-bold text-white">Compliance access is restricted</h2>
          <p className="text-xs text-slate-400 mt-2">
            {getDeniedMessage(session?.role, 'view_compliance')} {getRoleLabel(session?.role)} users can continue using Documents, Assets, and Copilot.
          </p>
          <Link
            href="/documents"
            className="mt-5 inline-flex items-center justify-center px-4 py-2 rounded-xl bg-slate-900 border border-slate-800 text-xs font-semibold text-slate-300 hover:text-cyber-emerald hover:border-cyber-emerald/30"
          >
            Open Document Hub
          </Link>
        </div>
      </NavigationShell>
    );
  }

  return (
    <NavigationShell>
      <div className="space-y-6">

        {/* Header Summary widgets */}
        <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
          <div className="glass-panel p-4 rounded-xl flex items-center gap-4">
            <div className="p-3 bg-rose-500/10 text-cyber-rose rounded-lg border border-cyber-rose/20">
              <ShieldAlert className="w-5 h-5" />
            </div>
            <div>
              <span className="text-[10px] text-slate-500 font-mono uppercase tracking-wider block">Open Regulatory Gaps</span>
              <span className="text-xl font-bold text-white">{openGapsCount} Gaps</span>
            </div>
          </div>

          <div className="glass-panel p-4 rounded-xl flex items-center gap-4">
            <div className="p-3 bg-amber-500/10 text-cyber-amber rounded-lg border border-cyber-amber/20">
              <Award className="w-5 h-5" />
            </div>
            <div>
              <span className="text-[10px] text-slate-500 font-mono uppercase tracking-wider block">Expired Certificates</span>
              <span className="text-xl font-bold text-white">{expiredCertsCount} Certificates</span>
            </div>
          </div>

          <div className="glass-panel p-4 rounded-xl flex items-center gap-4">
            <div className="p-3 bg-emerald-500/10 text-cyber-emerald rounded-lg border border-cyber-emerald/20">
              <CheckCircle2 className="w-5 h-5" />
            </div>
            <div>
              <span className="text-[10px] text-slate-500 font-mono uppercase tracking-wider block">Compliance Audit Score</span>
              <span className="text-xl font-bold text-white">
                {Math.round(((certificates.filter(c => c.status === 'Active').length + complianceGaps.filter(g => g.status === 'Closed').length) / (certificates.length + complianceGaps.length)) * 100)}%
              </span>
            </div>
          </div>
        </div>

        <div className="glass-panel p-4 rounded-2xl border-slate-800/80 flex flex-col sm:flex-row sm:items-center justify-between gap-4">
          <div>
            <h3 className="text-sm font-bold text-white tracking-wide">Evidence Scan</h3>
            <p className="text-xs text-slate-400 mt-0.5">Refresh missing inspections, certificates, SOP evidence, and procedure conflicts</p>
          </div>
          <button
            type="button"
            onClick={handleRunScan}
            disabled={scanBusy || !canScanCompliance}
            title={!canScanCompliance ? getDeniedMessage(session?.role, 'run_compliance_scan') : undefined}
            className="px-3 py-2 bg-cyber-emerald hover:bg-emerald-600 disabled:opacity-50 disabled:cursor-not-allowed text-slate-950 rounded-xl text-xs font-bold flex items-center justify-center gap-1.5 transition-all"
          >
            <RefreshCw className={`w-3.5 h-3.5 ${scanBusy ? 'animate-spin' : ''}`} />
            {scanBusy ? 'Scanning...' : 'Run Compliance Scan'}
          </button>
        </div>

        {actionMessage && (
          <div className="p-3 rounded-xl border border-cyber-amber/20 bg-cyber-amber/5 text-amber-200 text-xs flex gap-2">
            <AlertTriangle className="w-4 h-4 text-cyber-amber shrink-0" />
            <span>{actionMessage}</span>
          </div>
        )}

        {/* Tab Selection */}
        <div className="flex border-b border-slate-800 pb-px">
          <button
            onClick={() => setActiveTab('gaps')}
            className={`px-6 py-3 font-semibold text-xs uppercase tracking-wider transition-all border-b-2 -mb-px ${
              activeTab === 'gaps'
                ? 'text-cyber-emerald border-cyber-emerald bg-emerald-500/5'
                : 'text-slate-500 hover:text-slate-300 border-transparent'
            }`}
          >
            Regulatory Gaps ({openGapsCount})
          </button>
          <button
            onClick={() => setActiveTab('certificates')}
            className={`px-6 py-3 font-semibold text-xs uppercase tracking-wider transition-all border-b-2 -mb-px ${
              activeTab === 'certificates'
                ? 'text-cyber-emerald border-cyber-emerald bg-emerald-500/5'
                : 'text-slate-500 hover:text-slate-300 border-transparent'
            }`}
          >
            Certificates Registry ({certificates.length})
          </button>
        </div>

        {/* Tabs Content */}
        {activeTab === 'gaps' ? (
          <div className="glass-panel p-6 rounded-2xl border-slate-800/80 space-y-4">
            <div className="flex flex-col lg:flex-row lg:items-center justify-between gap-4 mb-4">
              <div>
                <h3 className="text-base font-bold text-white tracking-wide">Regulatory Gaps Inventory</h3>
                <p className="text-xs text-slate-400 mt-0.5">Identified conflicts, missing evidence records, and overdue inspections</p>
              </div>
              <div className="flex flex-wrap gap-2">
                <select
                  value={severityFilter}
                  onChange={(e) => setSeverityFilter(e.target.value)}
                  className="px-3 py-2 rounded-xl bg-slate-900/60 border border-slate-800 text-xs font-semibold text-slate-300 focus:outline-none focus:border-cyber-emerald"
                >
                  <option value="All">All Severities</option>
                  <option value="Critical">Critical</option>
                  <option value="High">High</option>
                  <option value="Medium">Medium</option>
                  <option value="Low">Low</option>
                </select>
                <select
                  value={statusFilter}
                  onChange={(e) => setStatusFilter(e.target.value)}
                  className="px-3 py-2 rounded-xl bg-slate-900/60 border border-slate-800 text-xs font-semibold text-slate-300 focus:outline-none focus:border-cyber-emerald"
                >
                  <option value="All">All Statuses</option>
                  <option value="Open">Open</option>
                  <option value="Closed">Closed</option>
                </select>
                <button
                  onClick={handleExport}
                  disabled={!canExportReports}
                  title={!canExportReports ? getDeniedMessage(session?.role, 'export_reports') : undefined}
                  className="px-3 py-2 bg-slate-800 hover:bg-slate-700 disabled:opacity-50 disabled:cursor-not-allowed border border-slate-700 text-slate-300 hover:text-white rounded-xl text-xs font-semibold flex items-center gap-1.5 transition-all"
                >
                  <FileDown className="w-3.5 h-3.5" /> Export CSV
                </button>
              </div>
            </div>

            <div className="space-y-4">
              {filteredGaps.length > 0 ? (
                filteredGaps.map((gap) => {
                  const isOpen = gap.status === 'Open';
                  return (
                    <div
                      key={gap.id}
                      className={`p-5 rounded-2xl border flex flex-col md:flex-row md:items-center justify-between gap-6 transition-all ${
                        isOpen
                          ? 'bg-[#090d1f]/40 border-slate-800 hover:border-slate-700/50'
                          : 'bg-emerald-500/5 border-emerald-500/10 opacity-70'
                      }`}
                    >
                      <div className="flex items-start gap-4 min-w-0">
                        <div className="mt-1 shrink-0">
                          {isOpen ? (
                            <AlertTriangle className={`w-5 h-5 ${
                              gap.severity === 'Critical' ? 'text-cyber-rose' :
                              gap.severity === 'High' ? 'text-cyber-amber' : 'text-cyber-blue'
                            }`} />
                          ) : (
                            <CheckCircle2 className="w-5 h-5 text-cyber-emerald" />
                          )}
                        </div>

                        <div className="space-y-1 min-w-0">
                          <div className="flex flex-wrap items-center gap-2">
                            <span className="text-xs font-bold text-slate-200">{gap.gapType}</span>
                            <span className={`text-[9px] font-mono font-bold uppercase border px-1.5 py-0.5 rounded-full ${
                              gap.severity === 'Critical' ? 'bg-cyber-rose/10 text-cyber-rose border-cyber-rose/20' :
                              gap.severity === 'High' ? 'bg-cyber-amber/10 text-cyber-amber border-cyber-amber/20' :
                              'bg-slate-800 text-slate-400 border-slate-700'
                            }`}>
                              {gap.severity}
                            </span>

                            <Link
                              href={`/assets/${gap.assetTag}`}
                              className="text-[9px] bg-slate-800 text-slate-400 border border-slate-700 px-2 py-0.5 rounded font-mono font-bold uppercase hover:text-cyber-emerald transition-colors"
                            >
                              {gap.assetTag}
                            </Link>
                          </div>

                          <p className="text-xs text-slate-400 leading-relaxed max-w-2xl">{gap.description}</p>
                          <span className="text-[10px] text-slate-500 font-mono block">Identified: {new Date(gap.createdAt).toLocaleDateString()}</span>
                        </div>
                      </div>

                      <div className="shrink-0 flex items-center gap-2">
                        {isOpen ? (
                          <button
                            onClick={() => {
                              if (!canResolveGap) {
                                setActionMessage(getDeniedMessage(session?.role, 'resolve_compliance_gap'));
                                return;
                              }
                              resolveGap(gap.id);
                            }}
                            disabled={!canResolveGap}
                            title={!canResolveGap ? getDeniedMessage(session?.role, 'resolve_compliance_gap') : undefined}
                            className="px-3 py-1.5 bg-cyber-emerald hover:bg-emerald-600 disabled:opacity-50 disabled:cursor-not-allowed active:bg-emerald-700 text-slate-950 font-bold rounded-xl text-xs flex items-center gap-1 transition-all hover:shadow-[0_0_15px_rgba(16,185,129,0.2)]"
                          >
                            <Check className="w-3.5 h-3.5" /> Resolve Gap
                          </button>
                        ) : (
                          <span className="text-[10px] text-cyber-emerald font-mono font-semibold bg-emerald-500/10 px-2.5 py-1 rounded-full border border-emerald-500/20 flex items-center gap-1">
                            <Check className="w-3.5 h-3.5" /> Gap Resolved
                          </span>
                        )}
                      </div>
                    </div>
                  );
                })
              ) : (
                <div className="py-12 text-center text-slate-500 font-mono text-xs border border-dashed border-slate-800 rounded-xl">
                  No gaps match the selected filters.
                </div>
              )}
            </div>
          </div>
        ) : (
          <div className="glass-panel p-6 rounded-2xl border-slate-800/80 space-y-4">
            <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-4">
              <div>
                <h3 className="text-base font-bold text-white tracking-wide">Certificates Registry</h3>
                <p className="text-xs text-slate-400 mt-0.5">Track regulatory pressure testing, loops calibration, and thick gauges checks</p>
              </div>
              <button
                onClick={handleExport}
                disabled={!canExportReports}
                title={!canExportReports ? getDeniedMessage(session?.role, 'export_reports') : undefined}
                className="px-3 py-2 bg-slate-800 hover:bg-slate-700 disabled:opacity-50 disabled:cursor-not-allowed border border-slate-700 text-slate-300 hover:text-white rounded-xl text-xs font-semibold flex items-center gap-1.5 transition-all self-start sm:self-auto"
              >
                <FileDown className="w-3.5 h-3.5" /> Export CSV
              </button>
            </div>

            <div className="overflow-x-auto">
              <table className="w-full text-left border-collapse">
                <thead>
                  <tr className="border-b border-slate-800/80 text-[10px] font-mono text-slate-500 uppercase tracking-widest">
                    <th className="py-3 px-4">Certificate Name</th>
                    <th className="py-3 px-4">Associated Asset</th>
                    <th className="py-3 px-4">Expiry Date</th>
                    <th className="py-3 px-4 text-center">Status</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-slate-800/40 text-xs">
                  {certificates.length > 0 ? (
                    certificates.map((cert) => {
                      const isExpired = cert.status === 'Expired' || cert.status === 'Overdue';
                      return (
                        <tr key={cert.id} className="hover:bg-slate-800/10 transition-colors">
                          <td className="py-4 px-4 font-semibold text-slate-200">
                            <div className="flex items-center gap-2.5">
                              <Award className={`w-4 h-4 ${isExpired ? 'text-cyber-rose' : 'text-cyber-emerald'}`} />
                              {cert.name}
                            </div>
                          </td>
                          <td className="py-4 px-4 font-mono">
                            <Link
                              href={`/assets/${cert.assetTag}`}
                              className="bg-slate-900/60 border border-slate-800 px-2 py-0.5 rounded text-[10px] hover:text-cyber-emerald transition-colors uppercase font-bold"
                            >
                              {cert.assetTag}
                            </Link>
                          </td>
                          <td className="py-4 px-4 font-mono text-slate-400">{cert.expiryDate}</td>
                          <td className="py-4 px-4 text-center whitespace-nowrap">
                            <span className={`text-[10px] font-mono font-bold uppercase border px-2.5 py-0.5 rounded-full ${
                              cert.status === 'Expired' ? 'bg-cyber-rose/10 text-cyber-rose border-cyber-rose/20' :
                              cert.status === 'Overdue' ? 'bg-cyber-amber/10 text-cyber-amber border-cyber-amber/20' :
                              'bg-cyber-emerald/10 text-cyber-emerald border-cyber-emerald/20'
                            }`}>
                              {cert.status}
                            </span>
                          </td>
                        </tr>
                      );
                    })
                  ) : (
                    <tr>
                      <td colSpan={4} className="py-12 text-center text-slate-500 font-mono text-xs">
                        No certificates registry matches found.
                      </td>
                    </tr>
                  )}
                </tbody>
              </table>
            </div>
          </div>
        )}

      </div>
    </NavigationShell>
  );
}

function exportComplianceCsv(
  activeTab: 'gaps' | 'certificates',
  gaps: ReturnType<typeof useData>['complianceGaps'],
  certificates: ReturnType<typeof useData>['certificates'],
) {
  const stamp = new Date().toISOString().slice(0, 10);
  const content = activeTab === 'gaps'
    ? [
        'asset_tag,gap_type,severity,status,description,evidence_document_id,created_at',
        ...gaps.map(gap => [
          gap.assetTag,
          gap.gapType,
          gap.severity,
          gap.status,
          gap.description,
          gap.evidenceDocId || '',
          gap.createdAt,
        ].map(csvEscape).join(',')),
      ].join('\n')
    : [
        'asset_tag,certificate_name,expiry_date,status',
        ...certificates.map(cert => [
          cert.assetTag,
          cert.name,
          cert.expiryDate,
          cert.status,
        ].map(csvEscape).join(',')),
      ].join('\n');

  const blob = new Blob([content], { type: 'text/csv;charset=utf-8' });
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = `compliance_${activeTab}_${stamp}.csv`;
  link.click();
  URL.revokeObjectURL(url);
}

function csvEscape(value: string | number) {
  const text = String(value ?? '');
  if (text.includes(',') || text.includes('"') || text.includes('\n')) {
    return `"${text.replace(/"/g, '""')}"`;
  }
  return text;
}
