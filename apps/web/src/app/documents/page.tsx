'use client';

import React, { useEffect, useMemo, useState } from 'react';
import NavigationShell from '../../components/NavigationShell';
import { useData } from '../../context/DataContext';
import { apiFetch, getStoredSession, PlantBrainSession, readApiError } from '../../lib/api';
import { Document } from '../../lib/mockData';
import { formatStatusLabel } from '../../lib/normalizers';
import {
  canDownloadDocumentSource,
  canRunAction,
  getDeniedMessage,
  getDocumentAccessInfo,
  getDocumentSourceDeniedMessage,
  getRoleLabel,
} from '../../lib/permissions';
import {
  AlertTriangle,
  CheckCircle2,
  Clock,
  Cpu,
  FileCheck,
  FileText,
  Filter,
  Info,
  Download,
  Lock,
  RefreshCw,
  Search,
  Trash2,
  Upload,
} from 'lucide-react';

interface DocumentProcessingFields {
  processingJobId?: string | null;
  processingStatus?: string | null;
  processingProgress?: number | null;
  processingAttempts?: number | null;
  retryAvailable?: boolean | null;
  retryUrl?: string | null;
  processingError?: string | null;
  jobUpdatedAt?: string | null;
}

type OperationalDocument = Document & DocumentProcessingFields;

const docTypes = [
  'OEM Manual',
  'SOP',
  'Maintenance Work Order',
  'Inspection Report',
  'Incident Report',
  'Audit Report',
  'Safety Procedure',
  'P&ID',
  'Compliance Document',
  'Training Document',
  'Unknown',
];

const pipelinePhases: Document['status'][] = [
  'UPLOADED',
  'EXTRACTING_TEXT',
  'OCR_RUNNING',
  'CLASSIFYING',
  'CHUNKING',
  'EXTRACTING_ENTITIES',
  'GENERATING_EMBEDDINGS',
  'BUILDING_GRAPH',
  'COMPLETED',
];

export default function DocumentHubPage() {
  const { documents, uploadDocument, uploadDocumentVersion, retryDocumentProcessing, refreshDocumentStatus, archiveDocument } = useData();
  const [searchQuery, setSearchQuery] = useState('');
  const [selectedType, setSelectedType] = useState('All');
  const [selectedStatus, setSelectedStatus] = useState('All');
  const [selectedDocId, setSelectedDocId] = useState<string | null>(documents[0]?.id || null);
  const [busyAction, setBusyAction] = useState<'download' | 'archive' | 'version' | 'retry' | 'refresh' | null>(null);
  const [actionError, setActionError] = useState('');

  const [dragActive, setDragActive] = useState(false);
  const [selectedFile, setSelectedFile] = useState<File | null>(null);
  const [docType, setDocType] = useState('OEM Manual');
  const [title, setTitle] = useState('');
  const [plantName, setPlantName] = useState('Unit-2 Plant');
  const [department, setDepartment] = useState('Mechanical Maintenance');
  const [version, setVersion] = useState('v1');
  const [assetTag, setAssetTag] = useState('');
  const [showForm, setShowForm] = useState(false);
  const [uploading, setUploading] = useState(false);
  const [uploadError, setUploadError] = useState('');
  const [session, setSession] = useState<PlantBrainSession | null>(null);

  useEffect(() => {
    setSession(getStoredSession());
  }, []);

  const canArchiveDocuments = canRunAction(session?.role, 'archive_document');

  const filteredDocs = useMemo(() => {
    const query = searchQuery.toLowerCase().trim();

    return documents.filter(doc => {
      const access = getDocumentAccessInfo(doc);
      const matchesSearch = !query || [
        doc.title,
        doc.documentType,
        doc.plantName,
        doc.department,
        doc.uploadedBy,
        access.label,
        access.sensitivityLabel,
        ...access.allowedRoleLabels,
        ...(doc.assetTags || []),
      ].some(value => String(value || '').toLowerCase().includes(query));
      const matchesType = selectedType === 'All' || doc.documentType === selectedType;
      const matchesStatus = selectedStatus === 'All'
        || (selectedStatus === 'Processing' && isProcessing(doc))
        || (selectedStatus === 'Completed' && doc.status === 'COMPLETED')
        || (selectedStatus === 'Needs Review' && (doc.status === 'FAILED' || doc.status === 'PARTIAL_SUCCESS' || doc.ocrConfidence < 0.9));

      return matchesSearch && matchesType && matchesStatus;
    });
  }, [documents, searchQuery, selectedStatus, selectedType]);

  const activeDoc = documents.find(doc => doc.id === selectedDocId) || filteredDocs[0] || null;

  const handleDownloadDocument = async (document: Document) => {
    setActionError('');

    if (!canDownloadDocumentSource(session?.role, document)) {
      setActionError(getDocumentSourceDeniedMessage(session?.role, document));
      return;
    }

    setBusyAction('download');
    try {
      const res = await apiFetch(`/api/documents/${encodeURIComponent(document.id)}/download`);
      if (!res.ok) {
        if (document.id.startsWith('doc_')) {
          throw new Error('This demo document does not have an uploaded source file.');
        }
        throw new Error(await readApiError(res, 'Unable to create signed download link'));
      }

      const body = await res.json() as { downloadUrl?: string };
      if (!body.downloadUrl) {
        throw new Error('Download link was not returned by the API.');
      }
      window.open(body.downloadUrl, '_blank', 'noopener,noreferrer');
    } catch (error) {
      setActionError(error instanceof Error ? error.message : 'Unable to download document');
    } finally {
      setBusyAction(null);
    }
  };

  const handleArchiveDocument = async (document: Document) => {
    setActionError('');

    if (!canArchiveDocuments) {
      setActionError(getDeniedMessage(session?.role, 'archive_document'));
      return;
    }

    setBusyAction('archive');
    try {
      await archiveDocument(document.id);
      if (selectedDocId === document.id) {
        setSelectedDocId(null);
      }
    } catch (error) {
      setActionError(error instanceof Error ? error.message : 'Unable to archive document');
    } finally {
      setBusyAction(null);
    }
  };

  const handleVersionUpload = async (document: Document, file: File, versionLabel: string) => {
    setActionError('');
    setBusyAction('version');
    try {
      await uploadDocumentVersion(document.id, file, {
        title: document.title,
        version: versionLabel,
      });
    } catch (error) {
      setActionError(error instanceof Error ? error.message : 'Unable to upload document version');
    } finally {
      setBusyAction(null);
    }
  };

  const handleRetryProcessing = async (document: Document) => {
    setActionError('');
    setBusyAction('retry');
    try {
      await retryDocumentProcessing(document.id);
    } catch (error) {
      setActionError(error instanceof Error ? error.message : 'Unable to retry document processing');
    } finally {
      setBusyAction(null);
    }
  };

  const handleRefreshProcessing = async (document: Document) => {
    setActionError('');
    setBusyAction('refresh');
    try {
      await refreshDocumentStatus(document.id);
    } catch (error) {
      setActionError(error instanceof Error ? error.message : 'Unable to refresh processing status');
    } finally {
      setBusyAction(null);
    }
  };

  const handleDrag = (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    setDragActive(e.type === 'dragenter' || e.type === 'dragover');
  };

  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    setDragActive(false);
    setUploadError('');

    if (e.dataTransfer.files?.[0]) {
      prepareFile(e.dataTransfer.files[0]);
    }
  };

  const handleFileSelect = (e: React.ChangeEvent<HTMLInputElement>) => {
    setUploadError('');
    if (e.target.files?.[0]) {
      prepareFile(e.target.files[0]);
    }
  };

  const prepareFile = (file: File) => {
    setSelectedFile(file);
    setTitle(file.name.replace(/\.[^.]+$/, ''));
    setShowForm(true);
  };

  const handleUploadSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!selectedFile) return;

    if (selectedFile.size === 0) {
      setUploadError('Empty files cannot be ingested.');
      return;
    }

    if (selectedFile.size > 50 * 1024 * 1024) {
      setUploadError('Maximum supported file size is 50 MB.');
      return;
    }

    setUploading(true);
    setUploadError('');
    try {
      await uploadDocument(selectedFile, docType, {
        title,
        plantName,
        department,
        version,
        assetTag,
      });

      setSelectedFile(null);
      setShowForm(false);
      setTitle('');
      setAssetTag('');
      setVersion('v1');
    } catch (error) {
      setUploadError(error instanceof Error ? error.message : 'Unable to upload document.');
    } finally {
      setUploading(false);
    }
  };

  return (
    <NavigationShell>
      <div className="grid grid-cols-1 xl:grid-cols-3 gap-6 lg:gap-8">
        <div className="xl:col-span-1 space-y-6">
          <div className="glass-panel p-5 sm:p-6 rounded-2xl border-slate-800/80">
            <h3 className="text-base font-bold text-white tracking-wide mb-4">Ingest Document</h3>
            <div className="mb-4 p-3 rounded-xl bg-emerald-500/5 border border-emerald-500/10 text-[11px] text-slate-300 flex items-start gap-2">
              <Info className="w-4 h-4 text-cyber-emerald shrink-0 mt-0.5" />
              <span>
                Upload is enabled for {getRoleLabel(session?.role)}. Restricted evidence actions are still governed by role.
              </span>
            </div>

            <form onSubmit={handleUploadSubmit} className="space-y-4">
              <div
                onDragEnter={handleDrag}
                onDragOver={handleDrag}
                onDragLeave={handleDrag}
                onDrop={handleDrop}
                className={`border-2 border-dashed rounded-xl p-6 sm:p-8 flex flex-col items-center justify-center text-center transition-all ${
                  dragActive
                    ? 'border-cyber-emerald bg-cyber-emerald/5'
                    : 'border-slate-800 bg-[#060918]/40 hover:border-slate-700/50'
                }`}
              >
                <Upload className="w-10 h-10 text-slate-500 mb-3" />
                <p className="text-xs font-semibold text-slate-200">PDF, DOCX, XLSX, CSV, image, TXT, or Markdown</p>
                <p className="text-[10px] text-slate-500 font-mono mt-1">Maximum file size: 50MB</p>

                <label className="mt-4 px-3 py-1.5 bg-slate-800/80 border border-slate-700 hover:border-slate-600 rounded-lg text-xs font-semibold cursor-pointer transition-all">
                  Browse Files
                  <input
                    type="file"
                    onChange={handleFileSelect}
                    className="hidden"
                    accept=".pdf,.xlsx,.csv,.docx,.png,.jpg,.jpeg,.txt,.md"
                  />
                </label>
              </div>

              {uploadError && (
                <div className="bg-cyber-rose/10 border border-cyber-rose/20 text-rose-300 p-3 rounded-xl flex items-start gap-2 text-xs">
                  <AlertTriangle className="w-4 h-4 shrink-0" />
                  <span>{uploadError}</span>
                </div>
              )}

              {showForm && (
                <div className="p-4 bg-[#060918]/80 border border-slate-800 rounded-xl space-y-4">
                  <div className="text-xs">
                    <span className="font-semibold text-slate-300 block truncate" title={selectedFile?.name}>{selectedFile?.name}</span>
                    <span className="text-[10px] text-slate-500 font-mono">
                      {selectedFile ? (selectedFile.size / (1024 * 1024)).toFixed(2) : '0.00'} MB
                    </span>
                  </div>

                  <Field label="Document Title">
                    <input
                      value={title}
                      onChange={(e) => setTitle(e.target.value)}
                      className="w-full px-3 py-2 rounded-lg glass-input text-xs font-medium"
                      required
                    />
                  </Field>

                  <Field label="Document Classification">
                    <select
                      value={docType}
                      onChange={(e) => setDocType(e.target.value)}
                      className="w-full px-3 py-2 rounded-lg glass-input text-xs font-medium"
                    >
                      {docTypes.map(type => (
                        <option key={type} value={type}>{type}</option>
                      ))}
                    </select>
                  </Field>

                  <div className="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-1 gap-3">
                    <Field label="Plant / Site">
                      <input
                        value={plantName}
                        onChange={(e) => setPlantName(e.target.value)}
                        className="w-full px-3 py-2 rounded-lg glass-input text-xs font-medium"
                      />
                    </Field>
                    <Field label="Department">
                      <input
                        value={department}
                        onChange={(e) => setDepartment(e.target.value)}
                        className="w-full px-3 py-2 rounded-lg glass-input text-xs font-medium"
                      />
                    </Field>
                    <Field label="Version">
                      <input
                        value={version}
                        onChange={(e) => setVersion(e.target.value)}
                        className="w-full px-3 py-2 rounded-lg glass-input text-xs font-medium"
                      />
                    </Field>
                    <Field label="Asset Tag">
                      <input
                        value={assetTag}
                        onChange={(e) => setAssetTag(e.target.value.toUpperCase())}
                        placeholder="P-101"
                        className="w-full px-3 py-2 rounded-lg glass-input text-xs font-medium"
                      />
                    </Field>
                  </div>

                  <div className="flex gap-2">
                    <button
                      type="button"
                      onClick={() => { setSelectedFile(null); setShowForm(false); setUploadError(''); }}
                      className="flex-1 py-2 border border-slate-800 hover:bg-slate-800/40 text-slate-400 font-semibold rounded-lg text-xs transition-all"
                    >
                      Cancel
                    </button>
                    <button
                      type="submit"
                      disabled={uploading}
                      className="flex-1 py-2 bg-cyber-emerald hover:bg-emerald-600 active:bg-emerald-700 text-slate-950 font-bold rounded-lg text-xs transition-all hover:shadow-[0_0_15px_rgba(16,185,129,0.2)] disabled:opacity-60"
                    >
                      {uploading ? 'Uploading...' : 'Upload & Ingest'}
                    </button>
                  </div>
                </div>
              )}
            </form>
          </div>

          <DocumentStatusPanel
            document={activeDoc}
            busyAction={busyAction}
            actionError={actionError}
            canArchive={canArchiveDocuments}
            role={session?.role}
            onDownload={handleDownloadDocument}
            onArchive={handleArchiveDocument}
            onVersionUpload={handleVersionUpload}
            onRetry={handleRetryProcessing}
            onRefresh={handleRefreshProcessing}
          />
        </div>

        <div className="xl:col-span-2 space-y-6">
          <div className="glass-panel p-5 sm:p-6 rounded-2xl border-slate-800/80">
            <div className="flex flex-col gap-4 mb-6">
              <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
                <div>
                  <h3 className="text-base font-bold text-white tracking-wide">Document Registry</h3>
                  <p className="text-xs text-slate-400 mt-0.5">Metadata, processing status, and linked asset evidence</p>
                </div>
                <div className="text-[10px] text-slate-500 font-mono">
                  {filteredDocs.length} of {documents.length} visible
                </div>
              </div>

              <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
                <div className="relative">
                  <Search className="w-3.5 h-3.5 text-slate-500 absolute left-3 top-1/2 -translate-y-1/2" />
                  <input
                    type="text"
                    value={searchQuery}
                    onChange={(e) => setSearchQuery(e.target.value)}
                    placeholder="Search documents, assets, plant..."
                    className="w-full pl-9 pr-4 py-2 rounded-xl bg-slate-900/60 border border-slate-800 hover:border-slate-700/50 text-xs font-semibold text-white focus:outline-none focus:border-cyber-emerald"
                  />
                </div>

                <div className="relative">
                  <Filter className="w-3.5 h-3.5 text-slate-500 absolute left-3 top-1/2 -translate-y-1/2" />
                  <select
                    value={selectedType}
                    onChange={(e) => setSelectedType(e.target.value)}
                    className="w-full pl-9 pr-8 py-2 rounded-xl bg-slate-900/60 border border-slate-800 hover:border-slate-700/50 text-xs font-semibold text-slate-300 focus:outline-none focus:border-cyber-emerald cursor-pointer appearance-none"
                  >
                    <option value="All">All Types</option>
                    {docTypes.map(type => (
                      <option key={type} value={type}>{type}</option>
                    ))}
                  </select>
                </div>

                <div className="relative">
                  <Clock className="w-3.5 h-3.5 text-slate-500 absolute left-3 top-1/2 -translate-y-1/2" />
                  <select
                    value={selectedStatus}
                    onChange={(e) => setSelectedStatus(e.target.value)}
                    className="w-full pl-9 pr-8 py-2 rounded-xl bg-slate-900/60 border border-slate-800 hover:border-slate-700/50 text-xs font-semibold text-slate-300 focus:outline-none focus:border-cyber-emerald cursor-pointer appearance-none"
                  >
                    <option value="All">All Statuses</option>
                    <option value="Processing">Processing</option>
                    <option value="Completed">Completed</option>
                    <option value="Needs Review">Needs Review</option>
                  </select>
                </div>
              </div>
            </div>

            <div className="hidden md:block overflow-x-auto">
              <table className="w-full text-left border-collapse">
                <thead>
                  <tr className="border-b border-slate-800/80 text-[10px] font-mono text-slate-500 uppercase tracking-widest">
                    <th className="py-3 px-4">Document Details</th>
                    <th className="py-3 px-4">Metadata</th>
                    <th className="py-3 px-4">Date Ingested</th>
                    <th className="py-3 px-4 text-center">Status</th>
                    <th className="py-3 px-4 text-center">Confidence</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-slate-800/40 text-xs">
                  {filteredDocs.length > 0 ? (
                    filteredDocs.map((doc) => (
                      <tr
                        key={doc.id}
                        onClick={() => setSelectedDocId(doc.id)}
                        className={`hover:bg-slate-800/10 transition-colors cursor-pointer ${
                          activeDoc?.id === doc.id ? 'bg-emerald-500/5' : ''
                        }`}
                      >
                        <td className="py-4 px-4 min-w-[240px]">
                          <DocumentTitle document={doc} />
                        </td>
                        <td className="py-4 px-4 min-w-[180px]">
                          <span className="font-medium text-slate-300 block">{doc.documentType}</span>
                          <span className="text-[10px] text-slate-500 font-mono">{doc.plantName || 'Unknown plant'} / {doc.department || 'Unassigned'}</span>
                          <AccessBadge document={doc} />
                        </td>
                        <td className="py-4 px-4 font-mono text-slate-500 whitespace-nowrap">{formatDate(doc.createdAt)}</td>
                        <td className="py-4 px-4 text-center whitespace-nowrap"><StatusPill document={doc} /></td>
                        <td className="py-4 px-4 text-center font-mono font-bold whitespace-nowrap"><Confidence document={doc} /></td>
                      </tr>
                    ))
                  ) : (
                    <tr>
                      <td colSpan={5} className="py-12 text-center text-slate-500 font-mono text-xs">
                        No documents matching the criteria were found.
                      </td>
                    </tr>
                  )}
                </tbody>
              </table>
            </div>

            <div className="md:hidden space-y-3">
              {filteredDocs.length > 0 ? filteredDocs.map(doc => (
                <button
                  type="button"
                  key={doc.id}
                  onClick={() => setSelectedDocId(doc.id)}
                  className={`w-full text-left p-4 rounded-xl border transition-all ${
                    activeDoc?.id === doc.id ? 'border-emerald-500/30 bg-emerald-500/5' : 'border-slate-800 bg-[#060918]/60'
                  }`}
                >
                  <DocumentTitle document={doc} />
                  <div className="mt-3 flex flex-wrap items-center gap-2">
                    <StatusPill document={doc} />
                    <AccessBadge document={doc} />
                    <span className="text-[10px] text-slate-500 font-mono">{doc.documentType}</span>
                    <span className="text-[10px] text-slate-500 font-mono">{formatDate(doc.createdAt)}</span>
                  </div>
                </button>
              )) : (
                <div className="py-12 text-center text-slate-500 font-mono text-xs">
                  No documents matching the criteria were found.
                </div>
              )}
            </div>
          </div>
        </div>
      </div>
    </NavigationShell>
  );
}

function DocumentStatusPanel({
  document,
  busyAction,
  actionError,
  canArchive,
  role,
  onDownload,
  onArchive,
  onVersionUpload,
  onRetry,
  onRefresh,
}: {
  document: Document | null;
  busyAction: 'download' | 'archive' | 'version' | 'retry' | 'refresh' | null;
  actionError: string;
  canArchive: boolean;
  role?: string;
  onDownload: (document: Document) => void;
  onArchive: (document: Document) => void;
  onVersionUpload: (document: Document, file: File, versionLabel: string) => void;
  onRetry: (document: Document) => void;
  onRefresh: (document: Document) => void;
}) {
  const [versionLabel, setVersionLabel] = useState('v2');
  useEffect(() => {
    setVersionLabel(bumpVersionLabel(document?.version || 'v1'));
  }, [document?.id, document?.version]);

  if (!document) {
    return (
      <div className="glass-panel p-6 rounded-2xl border-slate-800/80 text-center text-xs text-slate-500 font-mono">
        No document selected.
      </div>
    );
  }

  const operational = document as OperationalDocument;
  const phaseIndex = getPhaseIndex(document.status);
  const fallbackProgress = document.status === 'FAILED'
    ? 0
    : Math.max(8, Math.round(((phaseIndex + 1) / pipelinePhases.length) * 100));
  const progress = clampPercent(operational.processingProgress ?? fallbackProgress);
  const needsReview = document.status === 'FAILED' || document.status === 'PARTIAL_SUCCESS' || (document.ocrConfidence > 0 && document.ocrConfidence < 0.9);
  const access = getDocumentAccessInfo(document);
  const canDownloadSource = canDownloadDocumentSource(role, document);
  const downloadDisabled = busyAction !== null || !canDownloadSource;
  const archiveDisabled = busyAction !== null || !canArchive;
  const versionDisabled = busyAction !== null || !canDownloadSource;
  const retryAvailable = canRetryProcessing(document);
  const retryDisabled = busyAction !== null || !retryAvailable || document.id.startsWith('doc_') && document.status !== 'FAILED';
  const queueLabel = operational.processingStatus || (isProcessing(document) ? 'ACTIVE' : document.status);

  return (
    <div className="glass-panel p-5 sm:p-6 rounded-2xl border-slate-800/80 space-y-5">
      <div className="flex items-start justify-between gap-4">
        <div className="min-w-0">
          <h3 className="text-base font-bold text-white tracking-wide truncate">{document.title}</h3>
          <p className="text-[10px] text-slate-500 font-mono mt-1">{document.id} / {document.version || 'v1'}</p>
        </div>
        <StatusPill document={document} />
      </div>

      <div>
        <div className="flex items-center justify-between mb-2 text-[10px] font-mono text-slate-500 uppercase">
          <span>Processing</span>
          <span>{progress}%</span>
        </div>
        <div className="h-2 bg-slate-800 rounded-full overflow-hidden border border-slate-700">
          <div className={`h-full ${document.status === 'FAILED' ? 'bg-cyber-rose' : 'bg-cyber-emerald'}`} style={{ width: `${progress}%` }} />
        </div>
        <p className="text-[10px] text-slate-500 mt-2 font-mono">{formatStatusLabel(document.status)}</p>
      </div>

      <LifecycleSteps status={document.status} />

      <div className="grid grid-cols-1 sm:grid-cols-3 gap-3 text-xs">
        <Meta label="Queue Status" value={formatStatusLabel(queueLabel)} />
        <Meta label="Attempts" value={String(operational.processingAttempts ?? 0)} />
        <Meta label="Last Job Update" value={operational.jobUpdatedAt ? formatDateTime(operational.jobUpdatedAt) : 'Not reported'} />
      </div>

      {operational.processingError && (
        <div className="p-3 bg-cyber-rose/5 border border-cyber-rose/20 text-rose-200 rounded-xl text-xs flex items-start gap-2">
          <AlertTriangle className="w-4 h-4 shrink-0 text-cyber-rose" />
          <span>{operational.processingError}</span>
        </div>
      )}

      <div className="grid grid-cols-2 gap-3 text-xs">
        <Meta label="Type" value={document.documentType} />
        <Meta label="File" value={`${document.fileType} / ${document.size}`} />
        <Meta label="Plant" value={document.plantName || 'Unknown'} />
        <Meta label="Department" value={document.department || 'Unassigned'} />
        <Meta label="Uploaded By" value={document.uploadedBy || 'Demo User'} />
        <Meta label="Ingested" value={formatDate(document.createdAt)} />
        <Meta label="Access" value={access.label} />
        <Meta label="Sensitivity" value={access.sensitivityLabel} />
      </div>

      <div className="space-y-2">
        <span className="text-[10px] text-slate-500 font-mono uppercase tracking-wider">Asset Links</span>
        <div className="flex flex-wrap gap-2">
          {(document.assetTags || []).length > 0 ? document.assetTags?.map(tag => (
            <span key={tag} className="px-2.5 py-1 rounded-lg bg-slate-900 border border-slate-800 text-cyber-emerald text-[10px] font-mono font-bold">
              {tag}
            </span>
          )) : (
            <span className="text-xs text-slate-500">No asset tag detected.</span>
          )}
        </div>
      </div>

      <div className={`p-3 rounded-xl border text-xs flex items-start gap-2 ${
        canDownloadSource
          ? 'bg-emerald-500/5 border-emerald-500/20 text-emerald-100'
          : 'bg-slate-900/60 border-slate-800 text-slate-400'
      }`}>
        <Lock className={`w-4 h-4 shrink-0 mt-0.5 ${canDownloadSource ? 'text-cyber-emerald' : 'text-slate-500'}`} />
        <div>
          <span className="font-semibold text-slate-200 block">Source evidence: {canDownloadSource ? 'available' : 'metadata only'}</span>
          <span className="text-[11px] text-slate-400">
            {access.label} source downloads are limited to {access.allowedRoleLabels.join(', ')}.
          </span>
        </div>
      </div>

      <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
        <button
          type="button"
          onClick={() => onDownload(document)}
          disabled={downloadDisabled}
          title={!canDownloadSource ? getDocumentSourceDeniedMessage(role, document) : undefined}
          className="inline-flex items-center justify-center gap-2 px-3 py-2 rounded-xl bg-cyber-emerald hover:bg-emerald-600 disabled:opacity-60 text-slate-950 font-bold text-xs transition-all"
        >
          <Download className="w-4 h-4" />
          {busyAction === 'download' ? 'Preparing...' : 'Download Source'}
        </button>
        <label
          title={!canDownloadSource ? getDocumentSourceDeniedMessage(role, document) : undefined}
          className={`inline-flex items-center justify-center gap-2 px-3 py-2 rounded-xl border border-slate-700 text-slate-200 font-bold text-xs transition-all ${
            versionDisabled ? 'opacity-60 cursor-not-allowed' : 'hover:bg-slate-800/60 cursor-pointer'
          }`}
        >
          <Upload className="w-4 h-4" />
          {busyAction === 'version' ? 'Uploading...' : 'Upload New Version'}
          <input
            type="file"
            className="hidden"
            disabled={versionDisabled}
            accept=".pdf,.xlsx,.csv,.docx,.png,.jpg,.jpeg,.txt,.md"
            onChange={(event) => {
              const file = event.target.files?.[0];
              event.currentTarget.value = '';
              if (file) onVersionUpload(document, file, versionLabel);
            }}
          />
        </label>
        <button
          type="button"
          onClick={() => onArchive(document)}
          disabled={archiveDisabled}
          title={!canArchive ? getDeniedMessage(role, 'archive_document') : undefined}
          className="inline-flex items-center justify-center gap-2 px-3 py-2 rounded-xl border border-cyber-rose/30 text-rose-300 hover:bg-cyber-rose/10 disabled:opacity-60 font-bold text-xs transition-all"
        >
          <Trash2 className="w-4 h-4" />
          {busyAction === 'archive' ? 'Archiving...' : 'Archive'}
        </button>
        <button
          type="button"
          onClick={() => onRetry(document)}
          disabled={retryDisabled}
          className="inline-flex items-center justify-center gap-2 px-3 py-2 rounded-xl border border-cyber-amber/30 text-amber-200 hover:bg-cyber-amber/10 disabled:opacity-60 font-bold text-xs transition-all"
        >
          <RefreshCw className="w-4 h-4" />
          {busyAction === 'retry' ? 'Queueing...' : 'Retry Processing'}
        </button>
        <button
          type="button"
          onClick={() => onRefresh(document)}
          disabled={busyAction !== null || document.id.startsWith('doc_')}
          className="inline-flex items-center justify-center gap-2 px-3 py-2 rounded-xl border border-slate-700 text-slate-200 hover:bg-slate-800/60 disabled:opacity-60 font-bold text-xs transition-all"
        >
          <Clock className="w-4 h-4" />
          {busyAction === 'refresh' ? 'Refreshing...' : 'Refresh Status'}
        </button>
      </div>

      <Field label="Next Version Label">
        <input
          value={versionLabel}
          onChange={(event) => setVersionLabel(event.target.value)}
          disabled={versionDisabled}
          className="w-full px-3 py-2 rounded-lg glass-input text-xs font-medium disabled:opacity-60"
        />
      </Field>

      {actionError && (
        <div className="p-3 bg-cyber-rose/5 border border-cyber-rose/20 text-rose-200 rounded-xl text-xs flex items-start gap-2">
          <AlertTriangle className="w-4 h-4 shrink-0 text-cyber-rose" />
          <span>{actionError}</span>
        </div>
      )}

      {!canDownloadSource && (
        <div className="p-3 bg-slate-900/60 border border-slate-800 text-slate-400 rounded-xl text-xs flex items-start gap-2">
          <Info className="w-4 h-4 shrink-0 text-slate-500" />
          <span>{getRoleLabel(role)} can view metadata here, but source downloads for this document's evidence policy are restricted.</span>
        </div>
      )}

      {needsReview && (
        <div className="p-3 bg-cyber-amber/5 border border-cyber-amber/20 text-amber-200 rounded-xl text-xs flex items-start gap-2">
          <Info className="w-4 h-4 shrink-0 text-cyber-amber" />
          <span>Manual review recommended before using this document as compliance evidence.</span>
        </div>
      )}
    </div>
  );
}

function LifecycleSteps({ status }: { status: Document['status'] }) {
  const activeIndex = getPhaseIndex(status);
  const stages = [
    { label: 'Intake', index: 0 },
    { label: 'Extract', index: 1 },
    { label: 'Classify', index: 3 },
    { label: 'Embed', index: 6 },
    { label: 'Graph', index: 7 },
    { label: 'Ready', index: 8 },
  ];

  return (
    <div className="grid grid-cols-3 sm:grid-cols-6 gap-2">
      {stages.map(stage => {
        const complete = status === 'COMPLETED' || activeIndex >= stage.index;
        const failed = status === 'FAILED';
        return (
          <div
            key={stage.label}
            className={`px-2 py-2 rounded-lg border text-center text-[10px] font-mono font-semibold ${
              failed
                ? 'border-cyber-rose/20 bg-cyber-rose/5 text-rose-300'
                : complete
                  ? 'border-emerald-500/20 bg-emerald-500/5 text-cyber-emerald'
                  : 'border-slate-800 bg-slate-900/40 text-slate-600'
            }`}
          >
            {stage.label}
          </div>
        );
      })}
    </div>
  );
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div>
      <label className="block text-[10px] font-bold text-slate-400 uppercase tracking-wider mb-1.5">{label}</label>
      {children}
    </div>
  );
}

function DocumentTitle({ document }: { document: Document }) {
  const access = getDocumentAccessInfo(document);

  return (
    <div className="flex items-center gap-3 min-w-0">
      <FileText className={`w-4 h-4 shrink-0 ${isProcessing(document) ? 'text-cyber-amber animate-pulse' : access.sourceRestricted ? 'text-cyber-amber' : 'text-cyber-blue'}`} />
      <div className="min-w-0">
        <span className="font-semibold text-slate-200 block truncate" title={document.title}>{document.title}</span>
        <span className="text-[10px] text-slate-500 font-mono uppercase">
          {document.fileType} / {document.size}
        </span>
      </div>
    </div>
  );
}

function AccessBadge({ document }: { document: Document }) {
  const access = getDocumentAccessInfo(document);
  const restrictedClass = access.sourceRestricted
    ? 'bg-cyber-amber/10 text-cyber-amber border-cyber-amber/20'
    : 'bg-slate-900 text-slate-400 border-slate-800';

  return (
    <span className={`mt-1 inline-flex items-center gap-1 text-[9px] font-mono font-bold uppercase border px-2 py-0.5 rounded-full ${restrictedClass}`}>
      {access.sourceRestricted && <Lock className="w-2.5 h-2.5" />}
      {access.label} / {access.sensitivityLabel}
    </span>
  );
}

function StatusPill({ document }: { document: Document }) {
  const operational = document as OperationalDocument;
  const attempts = operational.processingAttempts && operational.processingAttempts > 0
    ? ` / try ${operational.processingAttempts}`
    : '';

  if (document.status === 'COMPLETED') {
    return (
      <span className="inline-flex items-center gap-1 text-[10px] bg-cyber-emerald/10 text-cyber-emerald border border-cyber-emerald/20 px-2.5 py-0.5 rounded-full font-mono font-bold">
        <CheckCircle2 className="w-3 h-3" /> Completed{attempts}
      </span>
    );
  }

  if (document.status === 'FAILED') {
    return (
      <span className="inline-flex items-center gap-1 text-[10px] bg-cyber-rose/10 text-cyber-rose border border-cyber-rose/20 px-2.5 py-0.5 rounded-full font-mono font-bold">
        <AlertTriangle className="w-3 h-3" /> Failed{attempts}
      </span>
    );
  }

  if (document.status === 'PARTIAL_SUCCESS') {
    return (
      <span className="inline-flex items-center gap-1 text-[10px] bg-cyber-amber/10 text-cyber-amber border border-cyber-amber/20 px-2.5 py-0.5 rounded-full font-mono font-bold">
        <FileCheck className="w-3 h-3" /> Partial{attempts}
      </span>
    );
  }

  return (
    <span className="inline-flex items-center gap-1 text-[10px] bg-cyber-amber/10 text-cyber-amber border border-cyber-amber/20 px-2.5 py-0.5 rounded-full font-mono font-semibold animate-pulse">
      <Cpu className="w-3 h-3" /> {formatStatusLabel(operational.processingStatus || document.status)}{attempts}
    </span>
  );
}

function Confidence({ document }: { document: Document }) {
  if (document.status !== 'COMPLETED' && document.status !== 'PARTIAL_SUCCESS') {
    return <span className="text-slate-600">-</span>;
  }

  return (
    <div className="flex flex-col items-center">
      <span className={document.ocrConfidence < 0.9 ? 'text-cyber-amber' : 'text-cyber-emerald'}>
        {(document.ocrConfidence * 100).toFixed(0)}% <span className="text-[9px] text-slate-500">OCR</span>
      </span>
      <span className="text-cyber-indigo text-[10px]">
        {(document.classificationConfidence * 100).toFixed(0)}% <span className="text-[9px] text-slate-500">CLS</span>
      </span>
    </div>
  );
}

function Meta({ label, value }: { label: string; value: string }) {
  return (
    <div className="p-3 bg-[#060918]/60 border border-slate-800 rounded-xl min-w-0">
      <span className="text-[10px] text-slate-500 font-mono uppercase tracking-wider block">{label}</span>
      <span className="text-xs text-slate-300 font-semibold truncate block mt-1" title={value}>{value}</span>
    </div>
  );
}

function isProcessing(document: Document) {
  return !['COMPLETED', 'FAILED', 'PARTIAL_SUCCESS'].includes(document.status);
}

function canRetryProcessing(document: Document) {
  const operational = document as OperationalDocument;
  return Boolean(
    operational.retryAvailable
      || operational.retryUrl
      || document.status === 'FAILED'
      || document.status === 'PARTIAL_SUCCESS'
      || operational.processingStatus === 'FAILED'
  );
}

function clampPercent(value: number) {
  if (!Number.isFinite(value)) return 0;
  return Math.max(0, Math.min(100, Math.round(value)));
}

function getPhaseIndex(status: Document['status']) {
  if (status === 'FAILED') return 0;
  if (status === 'PARTIAL_SUCCESS') return pipelinePhases.length - 2;
  const index = pipelinePhases.indexOf(status);
  return index >= 0 ? index : 0;
}

function formatDate(value: string) {
  return new Date(value).toLocaleDateString(undefined, {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  });
}

function formatDateTime(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return 'Invalid date';
  return date.toLocaleString(undefined, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  });
}

function bumpVersionLabel(version: string) {
  const match = /^v(\d+)$/i.exec(String(version || '').trim());
  if (!match) return 'v2';
  return `v${Number(match[1]) + 1}`;
}
