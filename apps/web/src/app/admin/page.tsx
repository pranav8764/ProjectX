'use client';

import React, { useEffect, useMemo, useState } from 'react';
import NavigationShell from '../../components/NavigationShell';
import { useData } from '../../context/DataContext';
import { apiFetch, appendPlantQuery, getStoredSession, PlantBrainSession, readApiError } from '../../lib/api';
import {
  canRunAction,
  getDeniedMessage,
  getRoleDocumentAccessSummary,
  getRoleLabel,
  isDocumentSourceRestricted,
  ROLE_DEFINITIONS,
  RoleKey,
} from '../../lib/permissions';
import {
  Activity,
  AlertTriangle,
  CheckCircle2,
  Clock,
  Database,
  Gauge,
  Lock,
  RefreshCw,
  Server,
  Settings,
  ShieldCheck,
  SlidersHorizontal,
  UserCheck,
  XCircle,
} from 'lucide-react';

type ProbeState = 'checking' | 'online' | 'offline' | 'restricted';

interface ServiceProbe {
  label: string;
  path: string;
  state: ProbeState;
  detail: string;
  latencyMs?: number;
}

interface DashboardMetrics {
  totalDocuments?: number;
  documentsProcessed?: number;
  documentsFailed?: number;
  assetsDiscovered?: number;
  criticalAssets?: number;
  repeatedFailures?: number;
  complianceGaps?: number;
  querySuccessRate?: number;
  averageQueryConfidence?: number;
  knowledgeGraphRelationships?: number;
  knowledgeGraphCompleteness?: number;
  timeSavedEstimateMinutes?: number;
  documentProcessingStatus?: Record<string, number>;
  processingQueueDepth?: number;
  processingRetryableDocuments?: number;
  processingAttempts?: number;
}

interface DocumentProcessingFields {
  processingStatus?: string | null;
  processingAttempts?: number | null;
  retryAvailable?: boolean | null;
  retryUrl?: string | null;
}

const initialProbes: ServiceProbe[] = [
  { label: 'API Gateway', path: '/api-health', state: 'checking', detail: 'Checking Go API health' },
  { label: 'AI Service', path: '/ai-health', state: 'checking', detail: 'Checking FastAPI worker health' },
  { label: 'Dashboard Metrics', path: '/api/dashboard/metrics', state: 'checking', detail: 'Checking metrics authorization' },
];

const settingRows = [
  { label: 'Require citations on AI answers', detail: 'Document-backed answers must return evidence before they are treated as usable.' },
  { label: 'Human approval for critical recommendations', detail: 'Critical maintenance and compliance recommendations stay advisory until reviewed.' },
  { label: 'Signed source document downloads', detail: 'Source files use expiring links and role checks for restricted evidence.' },
  { label: 'Audit log for document access', detail: 'Document reads, downloads, archives, and compliance actions remain traceable.' },
];

export default function AdminSettingsPage() {
  const { documents, assets, complianceGaps } = useData();
  const [session, setSession] = useState<PlantBrainSession | null>(null);
  const [remoteSession, setRemoteSession] = useState<Partial<PlantBrainSession> | null>(null);
  const [probes, setProbes] = useState<ServiceProbe[]>(initialProbes);
  const [metrics, setMetrics] = useState<DashboardMetrics | null>(null);
  const [metricsError, setMetricsError] = useState('');

  useEffect(() => {
    const stored = getStoredSession();
    setSession(stored);

    let cancelled = false;

    const loadAdminState = async () => {
      const meRes = await apiFetch('/api/me').catch(() => null);
      if (meRes?.ok) {
        const data = await meRes.json();
        if (!cancelled) setRemoteSession(data);
      }

      const [apiProbe, aiProbe, metricsProbe] = await Promise.all([
        probeService('/api-health', 'API Gateway'),
        probeService('/ai-health', 'AI Service'),
        probeService(appendPlantQuery('/api/dashboard/metrics'), 'Dashboard Metrics', true),
      ]);

      if (cancelled) return;

      setProbes([apiProbe.probe, aiProbe.probe, metricsProbe.probe]);
      if (metricsProbe.metrics) {
        setMetrics(metricsProbe.metrics);
        setMetricsError('');
      } else if (metricsProbe.probe.state === 'restricted') {
        setMetricsError(getDeniedMessage(stored?.role, 'view_dashboard'));
      } else if (metricsProbe.probe.state === 'offline') {
        setMetricsError(metricsProbe.probe.detail);
      }
    };

    loadAdminState().catch(() => {
      if (!cancelled) {
        setProbes(prev => prev.map(probe => ({ ...probe, state: 'offline', detail: 'Unable to reach service' })));
      }
    });

    return () => {
      cancelled = true;
    };
  }, []);

  const effectiveSession = useMemo(() => ({
    ...session,
    ...remoteSession,
    role: remoteSession?.role || session?.role,
    plantId: remoteSession?.plantId || session?.plantId,
    orgId: remoteSession?.orgId || session?.orgId,
  }), [remoteSession, session]);

  const canManageSettings = canRunAction(effectiveSession.role, 'manage_workspace_settings');
  const processingDocs = documents.filter(document => !['COMPLETED', 'FAILED', 'PARTIAL_SUCCESS'].includes(document.status)).length;
  const restrictedDocuments = documents.filter(isDocumentSourceRestricted).length;
  const failedDocs = metrics?.documentsFailed ?? documents.filter(document => document.status === 'FAILED').length;
  const retryableDocuments = metrics?.processingRetryableDocuments ?? documents.filter(isRetryableDocument).length;
  const processingAttempts = metrics?.processingAttempts ?? documents.reduce((sum, document) => sum + getProcessingAttempts(document), 0);
  const queueDepth = metrics?.processingQueueDepth ?? processingDocs;
  const graphCompleteness = metrics?.knowledgeGraphCompleteness;
  const querySuccess = metrics?.querySuccessRate;

  return (
    <NavigationShell>
      <div className="space-y-6">
        {!canManageSettings && (
          <div className="p-4 rounded-2xl border border-cyber-amber/20 bg-cyber-amber/5 text-amber-100 flex items-start gap-3">
            <Lock className="w-5 h-5 text-cyber-amber shrink-0 mt-0.5" />
            <div>
              <h3 className="text-sm font-bold text-white">Settings are read-only for {getRoleLabel(effectiveSession.role)}</h3>
              <p className="text-xs text-amber-100/80 mt-1">{getDeniedMessage(effectiveSession.role, 'manage_workspace_settings')}</p>
            </div>
          </div>
        )}

        <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
          <SummaryCard
            icon={<ShieldCheck className="w-5 h-5" />}
            tone="emerald"
            label="Session Role"
            value={getRoleLabel(effectiveSession.role)}
            detail={canManageSettings ? 'Settings write enabled' : 'Settings read-only'}
          />
          <SummaryCard
            icon={<Database className="w-5 h-5" />}
            tone="indigo"
            label="Plant Context"
            value={effectiveSession.plantId ? effectiveSession.plantId.slice(0, 12) : 'Offline demo'}
            detail={effectiveSession.plantName || effectiveSession.organizationName || 'Local workspace'}
          />
          <SummaryCard
            icon={<UserCheck className="w-5 h-5" />}
            tone="cyan"
            label="Auth Status"
            value={effectiveSession.userId ? 'Verified' : 'Local'}
            detail={effectiveSession.orgId ? `Org ${effectiveSession.orgId.slice(0, 10)}` : 'Demo token fallback'}
          />
        </div>

        <div className="grid grid-cols-1 xl:grid-cols-3 gap-6">
          <div className="xl:col-span-2 glass-panel p-6 rounded-2xl border-slate-800/80">
            <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-5">
              <div>
                <h3 className="text-base font-bold text-white tracking-wide">Role Matrix</h3>
                <p className="text-xs text-slate-400 mt-0.5">Frontend capability map aligned with backend role-gated endpoints</p>
              </div>
              <span className="text-[10px] bg-cyber-emerald/10 text-cyber-emerald border border-cyber-emerald/20 px-2.5 py-1 rounded-full font-mono font-bold flex items-center gap-1 self-start sm:self-auto">
                <CheckCircle2 className="w-3 h-3" /> Active
              </span>
            </div>

            <div className="overflow-x-auto">
              <table className="w-full text-left border-collapse text-xs">
                <thead>
                  <tr className="border-b border-slate-800/80 text-[10px] font-mono text-slate-500 uppercase tracking-widest">
                    <th className="py-3 px-4">Role</th>
                    <th className="py-3 px-4">Scope</th>
                    <th className="py-3 px-4 text-center">Archive</th>
                    <th className="py-3 px-4 text-center">Source</th>
                    <th className="py-3 px-4 text-center">RCA</th>
                    <th className="py-3 px-4 text-center">Compliance</th>
                    <th className="py-3 px-4 text-center">Reports</th>
                    <th className="py-3 px-4 text-center">Admin</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-slate-800/40">
                  {ROLE_DEFINITIONS.map(role => (
                    <tr key={role.key} className={`hover:bg-slate-800/10 ${getRoleLabel(effectiveSession.role) === role.label ? 'bg-emerald-500/5' : ''}`}>
                      <td className="py-4 px-4 font-bold text-slate-200 whitespace-nowrap">{role.label}</td>
                      <td className="py-4 px-4 text-slate-400 min-w-[260px]">
                        <span className="block">{role.scope}</span>
                        <span className="block text-[10px] text-slate-500 mt-1">Sources: {getRoleDocumentAccessSummary(role.key)}</span>
                      </td>
                      <CapabilityCell role={role.key} action="archive_document" />
                      <CapabilityCell role={role.key} action="download_restricted_document" />
                      <CapabilityCell role={role.key} action="generate_rca" />
                      <CapabilityCell role={role.key} action="run_compliance_scan" />
                      <CapabilityCell role={role.key} action="export_reports" />
                      <CapabilityCell role={role.key} action="manage_workspace_settings" />
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>

          <div className="glass-panel p-6 rounded-2xl border-slate-800/80">
            <div className="flex items-center justify-between mb-5">
              <div>
                <h3 className="text-base font-bold text-white tracking-wide">Workspace Controls</h3>
                <p className="text-xs text-slate-400 mt-0.5">Governance settings surfaced for demo review</p>
              </div>
              <Settings className="w-5 h-5 text-slate-500" />
            </div>

            <div className="space-y-3">
              {settingRows.map((row, index) => (
                <SettingRow
                  key={row.label}
                  label={row.label}
                  detail={row.detail}
                  enabled={index < 3}
                  disabled={!canManageSettings}
                />
              ))}
            </div>
          </div>
        </div>

        <div className="grid grid-cols-1 xl:grid-cols-3 gap-6">
          <div className="xl:col-span-1 glass-panel p-6 rounded-2xl border-slate-800/80">
            <div className="flex items-center justify-between mb-5">
              <div>
                <h3 className="text-base font-bold text-white tracking-wide">Service Observability</h3>
                <p className="text-xs text-slate-400 mt-0.5">Health probes for the consolidated MVP services</p>
              </div>
              <RefreshCw className="w-4 h-4 text-slate-500" />
            </div>

            <div className="space-y-3">
              {probes.map(probe => (
                <ServiceProbeRow key={probe.label} probe={probe} />
              ))}
            </div>

            {metricsError && (
              <div className="mt-4 p-3 bg-cyber-amber/5 border border-cyber-amber/20 rounded-xl text-xs text-amber-200 flex gap-2">
                <AlertTriangle className="w-4 h-4 text-cyber-amber shrink-0" />
                <span>{metricsError}</span>
              </div>
            )}
          </div>

          <div className="xl:col-span-2 glass-panel p-6 rounded-2xl border-slate-800/80">
            <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-5">
              <div>
                <h3 className="text-base font-bold text-white tracking-wide">Operational Metrics</h3>
                <p className="text-xs text-slate-400 mt-0.5">Backend metrics when available, otherwise local demo state</p>
              </div>
              <span className="text-[10px] bg-slate-900 text-slate-400 border border-slate-800 px-2.5 py-1 rounded-full font-mono font-bold self-start sm:self-auto">
                {metrics ? 'API metrics' : 'Local fallback'}
              </span>
            </div>

            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
              <MetricBox icon={<Clock className="w-4 h-4" />} label="Queue Depth" value={queueDepth} detail={`${retryableDocuments} retryable documents`} />
              <MetricBox icon={<AlertTriangle className="w-4 h-4" />} label="OCR Failures" value={failedDocs} detail="Failed ingestion jobs" />
              <MetricBox
                icon={<Gauge className="w-4 h-4" />}
                label="Query Success"
                value={querySuccess == null ? 'N/A' : `${Math.round(querySuccess * 100)}%`}
                detail={metrics?.averageQueryConfidence == null ? 'No backend query data' : `${Math.round(metrics.averageQueryConfidence * 100)}% avg confidence`}
              />
              <MetricBox
                icon={<Activity className="w-4 h-4" />}
                label="Graph Complete"
                value={graphCompleteness == null ? 'N/A' : `${Math.round(graphCompleteness * 100)}%`}
                detail={`${metrics?.knowledgeGraphRelationships ?? assets.reduce((sum, asset) => sum + asset.documents.length + asset.complianceGaps.length, 0)} visible links`}
              />
            </div>

            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4 mt-4">
              <MiniMetric label="Documents" value={metrics?.totalDocuments ?? documents.length} />
              <MiniMetric label="Assets" value={metrics?.assetsDiscovered ?? assets.length} />
              <MiniMetric label="Open Gaps" value={metrics?.complianceGaps ?? complianceGaps.filter(gap => gap.status === 'Open').length} />
              <MiniMetric label="Restricted Sources" value={restrictedDocuments} />
              <MiniMetric label="Retryable Docs" value={retryableDocuments} />
              <MiniMetric label="Job Attempts" value={processingAttempts} />
            </div>
          </div>
        </div>
      </div>
    </NavigationShell>
  );
}

async function probeService(path: string, label: string, expectMetrics = false): Promise<{ probe: ServiceProbe; metrics?: DashboardMetrics }> {
  const started = performance.now();
  try {
    const res = await apiFetch(path);
    const latencyMs = Math.round(performance.now() - started);

    if (res.status === 403) {
      return {
        probe: {
          label,
          path,
          state: 'restricted',
          detail: await readApiError(res, 'Current role cannot read this endpoint'),
          latencyMs,
        },
      };
    }

    if (!res.ok) {
      return {
        probe: {
          label,
          path,
          state: 'offline',
          detail: await readApiError(res, `HTTP ${res.status}`),
          latencyMs,
        },
      };
    }

    const body = await res.json().catch(() => ({}));
    return {
      probe: {
        label,
        path,
        state: 'online',
        detail: String(body.service || body.status || 'Reachable'),
        latencyMs,
      },
      metrics: expectMetrics ? body : undefined,
    };
  } catch {
    return {
      probe: {
        label,
        path,
        state: 'offline',
        detail: 'Service is not reachable from the web app',
      },
    };
  }
}

function SummaryCard({
  icon,
  tone,
  label,
  value,
  detail,
}: {
  icon: React.ReactNode;
  tone: 'emerald' | 'indigo' | 'cyan';
  label: string;
  value: string;
  detail: string;
}) {
  const toneClass = tone === 'emerald'
    ? 'bg-emerald-500/10 text-cyber-emerald border-emerald-500/20'
    : tone === 'indigo'
      ? 'bg-indigo-500/10 text-cyber-indigo border-indigo-500/20'
      : 'bg-cyan-500/10 text-cyan-400 border-cyan-500/20';

  return (
    <div className="glass-panel p-5 rounded-2xl border-slate-800/80">
      <div className="flex items-center gap-3">
        <div className={`p-3 rounded-xl border ${toneClass}`}>{icon}</div>
        <div className="min-w-0">
          <span className="text-[10px] text-slate-500 font-mono uppercase tracking-wider block">{label}</span>
          <span className="text-lg font-bold text-white block truncate">{value}</span>
          <span className="text-[10px] text-slate-500 block truncate">{detail}</span>
        </div>
      </div>
    </div>
  );
}

function CapabilityCell({ role, action }: { role: RoleKey; action: Parameters<typeof canRunAction>[1] }) {
  const enabled = canRunAction(role, action);
  return (
    <td className="py-4 px-4 text-center">
      {enabled ? (
        <CheckCircle2 className="w-4 h-4 text-cyber-emerald mx-auto" />
      ) : (
        <XCircle className="w-4 h-4 text-slate-700 mx-auto" />
      )}
    </td>
  );
}

function SettingRow({ label, detail, enabled, disabled }: { label: string; detail: string; enabled: boolean; disabled: boolean }) {
  return (
    <div className={`p-3 rounded-xl border bg-[#060918]/60 ${disabled ? 'border-slate-800 opacity-70' : 'border-slate-800 hover:border-slate-700/60'}`}>
      <div className="flex items-start justify-between gap-4">
        <div>
          <span className="text-xs font-bold text-slate-200">{label}</span>
          <p className="text-[10px] text-slate-500 mt-1 leading-relaxed">{detail}</p>
        </div>
        <div className={`w-10 h-5 rounded-full border p-0.5 shrink-0 ${enabled ? 'bg-emerald-500/20 border-emerald-500/30' : 'bg-slate-900 border-slate-800'}`}>
          <div className={`w-4 h-4 rounded-full ${enabled ? 'translate-x-5 bg-cyber-emerald' : 'translate-x-0 bg-slate-600'} transition-transform`} />
        </div>
      </div>
      {disabled && (
        <div className="mt-2 flex items-center gap-1.5 text-[10px] text-slate-500 font-mono">
          <Lock className="w-3 h-3" /> Admin required
        </div>
      )}
    </div>
  );
}

function ServiceProbeRow({ probe }: { probe: ServiceProbe }) {
  const stateClass = probe.state === 'online'
    ? 'text-cyber-emerald bg-emerald-500/10 border-emerald-500/20'
    : probe.state === 'restricted'
      ? 'text-cyber-amber bg-cyber-amber/10 border-cyber-amber/20'
      : probe.state === 'checking'
        ? 'text-cyan-400 bg-cyan-500/10 border-cyan-500/20'
        : 'text-cyber-rose bg-cyber-rose/10 border-cyber-rose/20';

  return (
    <div className="p-3 rounded-xl border border-slate-800 bg-[#060918]/60 flex items-center justify-between gap-3">
      <div className="flex items-center gap-3 min-w-0">
        <div className="p-2 rounded-lg bg-slate-900 border border-slate-800 text-slate-400">
          <Server className="w-4 h-4" />
        </div>
        <div className="min-w-0">
          <span className="text-xs font-bold text-slate-200 block">{probe.label}</span>
          <span className="text-[10px] text-slate-500 block truncate">{probe.detail}</span>
        </div>
      </div>
      <span className={`text-[9px] font-mono font-bold uppercase border px-2 py-0.5 rounded-full shrink-0 ${stateClass}`}>
        {probe.state}{probe.latencyMs ? ` ${probe.latencyMs}ms` : ''}
      </span>
    </div>
  );
}

function MetricBox({ icon, label, value, detail }: { icon: React.ReactNode; label: string; value: string | number; detail: string }) {
  return (
    <div className="p-4 rounded-xl border border-slate-800 bg-[#060918]/60">
      <div className="flex items-center justify-between gap-3">
        <div className="p-2 bg-emerald-500/10 text-cyber-emerald rounded-lg border border-emerald-500/20">
          {icon}
        </div>
        <SlidersHorizontal className="w-3.5 h-3.5 text-slate-700" />
      </div>
      <span className="text-[10px] text-slate-500 font-mono uppercase tracking-wider block mt-4">{label}</span>
      <span className="text-xl font-black text-white block mt-1">{value}</span>
      <p className="text-[10px] text-slate-500 mt-2">{detail}</p>
    </div>
  );
}

function MiniMetric({ label, value }: { label: string; value: number }) {
  return (
    <div className="p-3 rounded-xl border border-slate-800 bg-slate-900/40 flex items-center justify-between">
      <span className="text-[10px] text-slate-500 font-mono uppercase tracking-wider">{label}</span>
      <span className="text-sm font-bold text-slate-200">{value}</span>
    </div>
  );
}

function getOperationalDocument(document: unknown) {
  return document as DocumentProcessingFields & { status?: string };
}

function getProcessingAttempts(document: unknown) {
  const attempts = Number(getOperationalDocument(document).processingAttempts || 0);
  return Number.isFinite(attempts) ? attempts : 0;
}

function isRetryableDocument(document: unknown) {
  const operational = getOperationalDocument(document);
  return Boolean(
    operational.retryAvailable
      || operational.retryUrl
      || operational.status === 'FAILED'
      || operational.processingStatus === 'FAILED'
  );
}
