'use client';

import React, { useEffect, useState } from 'react';
import NavigationShell from '../../components/NavigationShell';
import { useData } from '../../context/DataContext';
import { apiFetch, getStoredSession, PlantBrainSession } from '../../lib/api';
import { canRunAction, getDeniedMessage } from '../../lib/permissions';
import { getQueryAnswerHistory, QueryAnswerRecord } from '../../lib/query-history';
import { Asset, ComplianceGap, Document } from '../../lib/mockData';
import { Activity, CheckCircle2, Clock, FileDown, FileSpreadsheet, FileText, MessageSquareText, ShieldAlert, AlertTriangle } from 'lucide-react';

type ReportType = 'asset_summary' | 'compliance_gap' | 'document_inventory' | 'rca_report' | 'query_answers';
type ReportFormat = 'csv' | 'pdf' | 'docx';

interface ReportJobResponse {
  id?: string;
  downloadUrl?: string;
  status?: string;
  error?: string;
}

const reportCards: Array<{ type: ReportType; title: string; icon: React.ReactNode }> = [
  { type: 'asset_summary', title: 'Asset Summary', icon: <FileText className="w-5 h-5" /> },
  { type: 'compliance_gap', title: 'Compliance Gaps', icon: <ShieldAlert className="w-5 h-5" /> },
  { type: 'document_inventory', title: 'Document Inventory', icon: <FileSpreadsheet className="w-5 h-5" /> },
  { type: 'rca_report', title: 'RCA Reports', icon: <Activity className="w-5 h-5" /> },
  { type: 'query_answers', title: 'Query Answers', icon: <MessageSquareText className="w-5 h-5" /> },
];

export default function ReportsPage() {
  const { assets, documents, complianceGaps } = useData();
  const [queuedReport, setQueuedReport] = useState<string | null>(null);
  const [busyType, setBusyType] = useState<ReportType | null>(null);
  const [queryHistory, setQueryHistory] = useState<QueryAnswerRecord[]>([]);
  const [reportFormat, setReportFormat] = useState<ReportFormat>('csv');

  const [session, setSession] = useState<PlantBrainSession | null>(null);

  useEffect(() => {
    setSession(getStoredSession());
    setQueryHistory(getQueryAnswerHistory());
  }, []);

  const canExport = canRunAction(session?.role, 'export_reports');

  const generateReport = async (type: ReportType) => {
    if (!canExport) return;
    setBusyType(type);
    let serverDownloadReady = false;

    try {
      const res = await apiFetch('/api/reports', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ reportType: type, format: reportFormat, parameters: { generatedFrom: 'web', format: reportFormat } }),
      });

      if (res.ok) {
        const job: ReportJobResponse = await res.json();
        setQueuedReport(job.id || null);
        if (job.downloadUrl) {
          serverDownloadReady = await downloadServerReport(job.downloadUrl, type, reportFormat);
        }
      }
    } catch {
      setQueuedReport(null);
    }

    if (!serverDownloadReady) {
      downloadClientReport(type, reportFormat, { assets, documents, complianceGaps, queryHistory });
    }
    setBusyType(null);
  };

  return (
    <NavigationShell>
      <div className="space-y-6">
        {!canExport && (
          <div className="p-4 bg-cyber-amber/5 border border-cyber-amber/20 text-amber-200 rounded-xl text-sm flex gap-2">
            <AlertTriangle className="w-5 h-5 shrink-0 text-cyber-amber" />
            <span>{getDeniedMessage(session?.role, 'export_reports')}</span>
          </div>
        )}

        <div className="glass-panel p-4 rounded-2xl border-slate-800/80 flex flex-col sm:flex-row sm:items-center justify-between gap-4">
          <div>
            <h3 className="text-sm font-bold text-white tracking-wide">Export Format</h3>
            <p className="text-xs text-slate-400 mt-0.5">Server reports support CSV, PDF, and DOCX artifacts</p>
          </div>
          <div className="inline-flex bg-slate-900/70 border border-slate-800 rounded-xl p-1">
            {(['csv', 'pdf', 'docx'] as ReportFormat[]).map(format => (
              <button
                type="button"
                key={format}
                onClick={() => setReportFormat(format)}
                className={`px-4 py-2 rounded-lg text-[10px] font-mono font-bold uppercase transition-all ${
                  reportFormat === format ? 'bg-cyber-emerald text-slate-950' : 'text-slate-400 hover:text-slate-200'
                }`}
              >
                {format}
              </button>
            ))}
          </div>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-3 xl:grid-cols-5 gap-6">
          {reportCards.map(card => (
            <div key={card.type} className="glass-panel p-5 rounded-2xl border-slate-800/80 space-y-5">
              <div className="flex items-center justify-between">
                <div className="p-3 bg-emerald-500/10 text-cyber-emerald rounded-xl border border-emerald-500/20">
                  {card.icon}
                </div>
                <span className="text-[10px] font-mono text-slate-500 uppercase">{reportFormat}</span>
              </div>
              <div>
                <h3 className="text-base font-bold text-white">{card.title}</h3>
                <p className="text-xs text-slate-400 mt-1">{getReportCount(card.type, assets, documents, complianceGaps, queryHistory)} records</p>
              </div>
              <button
                onClick={() => generateReport(card.type)}
                disabled={busyType === card.type || !canExport}
                className="w-full py-2.5 bg-cyber-emerald hover:bg-emerald-600 disabled:opacity-60 text-slate-950 font-bold rounded-xl text-xs transition-all flex items-center justify-center gap-2"
              >
                <FileDown className="w-4 h-4" />
                {busyType === card.type ? 'Generating...' : 'Export'}
              </button>
            </div>
          ))}
        </div>

        <div className="glass-panel p-6 rounded-2xl border-slate-800/80">
          <div className="flex items-center justify-between mb-5">
            <div>
              <h3 className="text-base font-bold text-white tracking-wide">Report Jobs</h3>
              <p className="text-xs text-slate-400 mt-0.5">Generated artifacts and queued backend jobs</p>
            </div>
            {queuedReport ? (
              <span className="text-[10px] bg-cyber-emerald/10 text-cyber-emerald border border-cyber-emerald/20 px-2.5 py-1 rounded-full font-mono font-bold flex items-center gap-1">
                <CheckCircle2 className="w-3 h-3" /> {queuedReport.slice(0, 8)}
              </span>
            ) : (
              <span className="text-[10px] bg-slate-800 text-slate-400 border border-slate-700 px-2.5 py-1 rounded-full font-mono font-bold flex items-center gap-1">
                <Clock className="w-3 h-3" /> Local export ready
              </span>
            )}
          </div>

          <div className="overflow-x-auto">
            <table className="w-full text-left border-collapse text-xs">
              <thead>
                <tr className="border-b border-slate-800/80 text-[10px] font-mono text-slate-500 uppercase tracking-widest">
                  <th className="py-3 px-4">Report</th>
                  <th className="py-3 px-4">Scope</th>
                  <th className="py-3 px-4">Status</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-800/40">
                {reportCards.map(card => (
                  <tr key={card.type} className="hover:bg-slate-800/10">
                    <td className="py-4 px-4 font-semibold text-slate-200">{card.title}</td>
                    <td className="py-4 px-4 text-slate-400">{getReportCount(card.type, assets, documents, complianceGaps, queryHistory)} records</td>
                    <td className="py-4 px-4">
                      <span className="text-[10px] bg-cyber-emerald/10 text-cyber-emerald border border-cyber-emerald/20 px-2.5 py-0.5 rounded-full font-mono font-bold">
                        Available
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

function getReportCount(type: ReportType, assets: Asset[], documents: Document[], gaps: ComplianceGap[], queryHistory: QueryAnswerRecord[]) {
  if (type === 'asset_summary') return assets.length;
  if (type === 'document_inventory') return documents.length;
  if (type === 'rca_report') return assets.reduce((count, asset) => count + asset.failures.length, 0);
  if (type === 'query_answers') return queryHistory.length;
  return gaps.length;
}

async function downloadServerReport(downloadUrl: string, type: ReportType, format: ReportFormat) {
  const res = await apiFetch(downloadUrl);
  if (!res.ok) return false;

  const blob = await res.blob();
  const disposition = res.headers.get('Content-Disposition');
  const filename = getFilename(disposition) || `${type}_${new Date().toISOString().slice(0, 10)}.${format}`;
  triggerDownload(blob, filename);
  return true;
}

function downloadClientReport(
  type: ReportType,
  format: ReportFormat,
  data: { assets: Asset[]; documents: Document[]; complianceGaps: ComplianceGap[]; queryHistory: QueryAnswerRecord[] }
) {
  const stamp = new Date().toISOString().slice(0, 10);
  const filename = `${type}_${stamp}.${format === 'csv' ? 'csv' : 'txt'}`;
  let content = '';

  if (type === 'asset_summary') {
    content = [
      'asset_tag,asset_name,asset_type,location,criticality,risk_score,failures,documents',
      ...data.assets.map(asset => [
        asset.assetTag,
        asset.assetName,
        asset.assetType,
        asset.location,
        asset.criticality,
        asset.riskScore,
        asset.failures.length,
        asset.documents.length,
      ].map(csvEscape).join(',')),
    ].join('\n');
  }

  if (type === 'compliance_gap') {
    content = [
      'asset_tag,gap_type,severity,status,description,created_at',
      ...data.complianceGaps.map(gap => [
        gap.assetTag,
        gap.gapType,
        gap.severity,
        gap.status,
        gap.description,
        gap.createdAt,
      ].map(csvEscape).join(',')),
    ].join('\n');
  }

  if (type === 'document_inventory') {
    content = [
      'title,file_type,document_type,status,ocr_confidence,classification_confidence,created_at',
      ...data.documents.map(doc => [
        doc.title,
        doc.fileType,
        doc.documentType,
        doc.status,
        doc.ocrConfidence,
        doc.classificationConfidence,
        doc.createdAt,
      ].map(csvEscape).join(',')),
    ].join('\n');
  }

  if (type === 'rca_report') {
    content = [
      'asset_tag,date,description,severity,status,work_order,maintenance_action',
      ...data.assets.flatMap(asset => asset.failures.map(failure => [
        asset.assetTag,
        failure.date,
        failure.description,
        failure.severity,
        failure.status,
        failure.workOrder,
        failure.maintenanceAction,
      ].map(csvEscape).join(','))),
    ].join('\n');
  }

  if (type === 'query_answers') {
    content = [
      'query,answer,confidence,asset_filter,citations,missing_info,created_at',
      ...data.queryHistory.map(record => [
        record.query,
        record.answer,
        record.confidence,
        record.assetFilter || '',
        record.citations.join('; '),
        record.missingInfo.join('; '),
        record.createdAt,
      ].map(csvEscape).join(',')),
    ].join('\n');
  }

  if (format !== 'csv') {
    content = `${type.replace(/_/g, ' ').toUpperCase()} REPORT\nGenerated: ${new Date().toISOString()}\n\n${content}`;
  }

  const blob = new Blob([content], { type: format === 'csv' ? 'text/csv;charset=utf-8' : 'text/plain;charset=utf-8' });
  triggerDownload(blob, filename);
}

function csvEscape(value: string | number) {
  const text = String(value ?? '');
  if (text.includes(',') || text.includes('"') || text.includes('\n')) {
    return `"${text.replace(/"/g, '""')}"`;
  }
  return text;
}

function getFilename(disposition: string | null) {
  if (!disposition) return null;
  const match = disposition.match(/filename="?([^";]+)"?/i);
  return match?.[1] || null;
}

function triggerDownload(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = filename;
  link.click();
  URL.revokeObjectURL(url);
}
