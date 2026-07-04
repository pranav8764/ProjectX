'use client';

import React, { createContext, useContext, useState, useEffect } from 'react';
import { 
  Document, 
  Asset, 
  ComplianceGap, 
  Certificate, 
  INITIAL_DOCUMENTS, 
  INITIAL_ASSETS, 
  INITIAL_COMPLIANCE_GAPS, 
  INITIAL_CERTIFICATES 
} from '../lib/mockData';
import { apiFetch, appendPlantQuery, getPlantId, getStoredSession, mergeStoredSession, readApiError } from '../lib/api';
import {
  normalizeCriticality,
  normalizeDocumentStatus,
  normalizeDocumentType,
  normalizeGapStatus,
  normalizeSeverity,
} from '../lib/normalizers';

export interface UploadMetadata {
  title?: string;
  plantId?: string;
  plantName?: string;
  department?: string;
  version?: string;
  assetTag?: string;
}

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

interface DataContextType {
  documents: Document[];
  assets: Asset[];
  complianceGaps: ComplianceGap[];
  certificates: Certificate[];
  uploadDocument: (file: File, type: string, metadata?: UploadMetadata) => Promise<void>;
  uploadDocumentVersion: (documentId: string, file: File, metadata?: Pick<UploadMetadata, 'title' | 'version'>) => Promise<void>;
  retryDocumentProcessing: (documentId: string) => Promise<void>;
  refreshDocumentStatus: (documentId: string) => Promise<void>;
  archiveDocument: (documentId: string) => Promise<void>;
  resolveGap: (gapId: string) => void;
  addFailureEvent: (assetTag: string, description: string, severity: 'Low' | 'Medium' | 'High' | 'Critical') => void;
}

const DataContext = createContext<DataContextType | undefined>(undefined);

export const DataProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [documents, setDocuments] = useState<Document[]>(INITIAL_DOCUMENTS);
  const [assets, setAssets] = useState<Asset[]>(INITIAL_ASSETS);
  const [complianceGaps, setComplianceGaps] = useState<ComplianceGap[]>(INITIAL_COMPLIANCE_GAPS);
  const [certificates, setCertificates] = useState<Certificate[]>(INITIAL_CERTIFICATES);

  // Load state from localStorage on mount (client side)
  useEffect(() => {
    const savedDocs = localStorage.getItem('plantbrain_documents');
    const savedAssets = localStorage.getItem('plantbrain_assets');
    const savedGaps = localStorage.getItem('plantbrain_compliance_gaps');
    const savedCerts = localStorage.getItem('plantbrain_certificates');

    safelyHydrate(savedDocs, setDocuments);
    safelyHydrate(savedAssets, setAssets);
    safelyHydrate(savedGaps, setComplianceGaps);
    safelyHydrate(savedCerts, setCertificates);

    let cancelled = false;

    const loadApiState = async () => {
      const meRes = await apiFetch('/api/me').catch(() => null);
      if (meRes?.ok) {
        const session = await meRes.json();
        mergeStoredSession({
          userId: session.userId,
          orgId: session.orgId,
          plantId: session.plantId,
          role: session.role || undefined,
        });
      }

      const [docsRes, assetsRes, gapsRes] = await Promise.all([
        apiFetch(appendPlantQuery('/api/documents')),
        apiFetch(appendPlantQuery('/api/assets')),
        apiFetch('/api/compliance/gaps'),
      ]);

      if (cancelled) return;

      let loadedDocuments: Document[] | null = null;
      let loadedGaps: ComplianceGap[] | null = null;

      if (docsRes.ok) {
        const apiDocs = await docsRes.json();
        const nextDocs = apiDocs.map((doc: any): Document & DocumentProcessingFields => ({
          id: doc.id,
          title: doc.title,
          fileType: doc.fileType || 'FILE',
          documentType: normalizeDocumentType(doc.documentType),
          status: normalizeDocumentStatus(doc.status),
          ocrConfidence: Number(doc.ocrConfidence || 0),
          classificationConfidence: Number(doc.classificationConfidence || 0),
          createdAt: doc.createdAt,
          size: doc.size || 'Stored',
          plantName: doc.plantName,
          department: doc.department,
          version: doc.version,
          accessLevel: doc.accessLevel,
          sensitivity: doc.sensitivity,
          allowedRoles: doc.allowedRoles,
          sourceRestricted: doc.sourceRestricted,
          sourceDownloadAllowed: doc.sourceDownloadAllowed,
          processingJobId: doc.processingJobId,
          processingStatus: doc.processingStatus,
          processingProgress: numberOrNull(doc.processingProgress),
          processingAttempts: numberOrNull(doc.processingAttempts),
          retryAvailable: doc.retryAvailable,
          retryUrl: doc.retryUrl,
          processingError: doc.processingError || doc.errorMessage,
          jobUpdatedAt: doc.jobUpdatedAt,
        }));
        loadedDocuments = nextDocs;
        setDocuments(nextDocs);

        await Promise.allSettled(
          nextDocs
            .filter((doc: Document & DocumentProcessingFields) => doc.id && !doc.id.startsWith('doc_') && (isRetryCandidate(doc) || isActiveProcessingStatus(doc.status)))
            .slice(0, 12)
            .map((doc: Document & DocumentProcessingFields) => refreshDocumentStatus(doc.id))
        );
      }

      if (gapsRes.ok) {
        const apiGaps = await gapsRes.json();
        const nextGaps = apiGaps.map((gap: any): ComplianceGap => ({
          id: gap.id,
          assetTag: gap.assetTag || 'UNKNOWN',
          gapType: normalizeDocumentType(gap.gapType || 'Compliance Gap'),
          description: gap.description,
          severity: normalizeSeverity(gap.severity),
          evidenceDocId: gap.evidenceDocumentId,
          status: normalizeGapStatus(gap.status),
          createdAt: gap.createdAt,
        }));
        loadedGaps = nextGaps;
        setComplianceGaps(nextGaps);
      }

      if (assetsRes.ok) {
        const apiAssets = await assetsRes.json();
        const nextAssets = apiAssets.map((asset: any): Asset => {
          const existing = INITIAL_ASSETS.find(a => a.assetTag === asset.assetTag);
          const linkedDocuments = (loadedDocuments || documents)
            .filter(doc => doc.title.toUpperCase().includes(asset.assetTag))
            .map(doc => doc.id);
          const linkedGaps = (loadedGaps || complianceGaps)
            .filter(gap => gap.assetTag === asset.assetTag)
            .map(gap => gap.id);

          return {
            id: asset.id,
            assetTag: asset.assetTag,
            assetName: asset.assetName || existing?.assetName || asset.assetTag,
            assetType: asset.assetType || existing?.assetType || 'Asset',
            location: asset.location || existing?.location || 'Plant',
            criticality: normalizeCriticality(asset.criticality || existing?.criticality),
            riskScore: Number(asset.riskScore || existing?.riskScore || 50),
            failures: existing?.failures || [],
            documents: linkedDocuments.length > 0 ? linkedDocuments : existing?.documents || [],
            complianceGaps: linkedGaps.length > 0 ? linkedGaps : existing?.complianceGaps || [],
          };
        });
        setAssets(nextAssets);
      }
    };

    loadApiState().catch(() => {
      // Offline demo mode keeps the seeded mock state available.
    });

    return () => {
      cancelled = true;
    };
  }, []);

  // Save state helper
  const saveState = (docs: Document[], asts: Asset[], gaps: ComplianceGap[], certs: Certificate[]) => {
    localStorage.setItem('plantbrain_documents', JSON.stringify(docs));
    localStorage.setItem('plantbrain_assets', JSON.stringify(asts));
    localStorage.setItem('plantbrain_compliance_gaps', JSON.stringify(gaps));
    localStorage.setItem('plantbrain_certificates', JSON.stringify(certs));
  };

  const uploadDocumentVersion = async (documentId: string, file: File, metadata: Pick<UploadMetadata, 'title' | 'version'> = {}) => {
    const existing = documents.find(doc => doc.id === documentId);
    if (!existing) {
      throw new Error('Document not found.');
    }
    if (file.size === 0) {
      throw new Error('Empty files cannot be ingested.');
    }
    if (file.size > 50 * 1024 * 1024) {
      throw new Error('Maximum supported file size is 50 MB.');
    }

    const formData = new FormData();
    formData.append('file', file);
    if (metadata.title?.trim()) formData.append('title', metadata.title.trim());
    if (metadata.version?.trim()) formData.append('versionLabel', metadata.version.trim());

    let uploaded: any = null;
    if (!documentId.startsWith('doc_')) {
      const res = await apiFetch(`/api/documents/${encodeURIComponent(documentId)}/versions`, {
        method: 'POST',
        body: formData,
      });
      if (!res.ok) {
        throw new Error(await readApiError(res, 'Unable to upload document version'));
      }
      uploaded = await res.json();
      if (uploaded.duplicate) {
        throw new Error(uploaded.message || 'This file already exists as a document version.');
      }
    }

    const nextVersion = uploaded?.versionLabel || metadata.version?.trim() || bumpVersionLabel(existing.version || 'v1');
    const nextTitle = metadata.title?.trim() || existing.title;
    setDocuments(prevDocs => {
      const nextDocs = prevDocs.map(doc => (
        doc.id === documentId
          ? {
              ...doc,
              title: nextTitle,
              fileType: file.name.split('.').pop()?.toUpperCase() || doc.fileType,
              size: file.size > 1024 * 1024 ? `${(file.size / (1024 * 1024)).toFixed(1)} MB` : `${(file.size / 1024).toFixed(0)} KB`,
              version: nextVersion,
              status: normalizeDocumentStatus(uploaded?.status || 'UPLOADED'),
              processingStatus: uploaded?.processingStatus || 'QUEUED',
              processingProgress: numberOrNull(uploaded?.processingProgress) ?? 0,
              processingAttempts: numberOrNull(uploaded?.processingAttempts),
              retryAvailable: Boolean(uploaded?.retryUrl),
              retryUrl: uploaded?.retryUrl,
              processingError: uploaded?.message && uploaded?.status === 'FAILED' ? uploaded.message : undefined,
              ocrConfidence: 0,
              classificationConfidence: 0,
              createdAt: new Date().toISOString(),
            }
          : doc
      ));
      saveState(nextDocs, assets, complianceGaps, certificates);
      return nextDocs;
    });
  };

  const uploadDocument = async (file: File, type: string, metadata: UploadMetadata = {}) => {
    const name = file.name;
    const sizeBytes = file.size;
    const newDocId = `doc_${Date.now()}`;
    let activeDocId = newDocId;
    const title = metadata.title?.trim() || name;
    const session = getStoredSession();
    const detectedAssetTags = [
      metadata.assetTag?.trim().toUpperCase(),
      ...assets
        .filter(a => name.toUpperCase().includes(a.assetTag))
        .map(a => a.assetTag),
    ].filter(Boolean) as string[];
    const formattedSize = sizeBytes > 1024 * 1024 
      ? `${(sizeBytes / (1024 * 1024)).toFixed(1)} MB`
      : `${(sizeBytes / 1024).toFixed(0)} KB`;
    
    const newDoc: Document = {
      id: newDocId,
      title,
      fileType: name.split('.').pop()?.toUpperCase() || 'PDF',
      documentType: normalizeDocumentType(type),
      status: 'UPLOADED',
      ocrConfidence: 0,
      classificationConfidence: 0,
      createdAt: new Date().toISOString(),
      size: formattedSize,
      plantName: metadata.plantName || 'Unit-2 Plant',
      department: metadata.department,
      version: metadata.version || 'v1',
      assetTags: Array.from(new Set(detectedAssetTags)),
      uploadedBy: session?.name || 'Demo User',
    };

    const updatedDocs = [newDoc, ...documents];
    setDocuments(updatedDocs);
    saveState(updatedDocs, assets, complianceGaps, certificates);

    const formData = new FormData();
    formData.append('file', file);
    formData.append('documentType', type);
    formData.append('title', title);
    if (metadata.plantId || getPlantId()) formData.append('plantId', metadata.plantId || getPlantId() || '');
    if (metadata.plantName) formData.append('plantName', metadata.plantName);
    if (metadata.department) formData.append('department', metadata.department);
    if (metadata.version) formData.append('version', metadata.version);
    if (metadata.assetTag) formData.append('assetTag', metadata.assetTag.toUpperCase());

    try {
      const res = await apiFetch('/api/documents/upload', {
        method: 'POST',
        body: formData,
      });

      if (res.ok) {
        const uploaded = await res.json();
        activeDocId = uploaded.documentId || newDocId;
        setDocuments(prevDocs => {
          const nextDocs = prevDocs.map(d => (
            d.id === newDocId
              ? {
                  ...d,
                  id: activeDocId,
                  status: normalizeDocumentStatus(uploaded.status || d.status),
                  processingStatus: uploaded.processingStatus || 'QUEUED',
                  processingProgress: numberOrNull(uploaded.processingProgress) ?? 0,
                  processingAttempts: numberOrNull(uploaded.processingAttempts),
                  retryAvailable: Boolean(uploaded.retryUrl),
                  retryUrl: uploaded.retryUrl,
                }
              : d.id === activeDocId
                ? {
                    ...d,
                    processingStatus: uploaded.processingStatus || 'QUEUED',
                    processingProgress: numberOrNull(uploaded.processingProgress) ?? 0,
                    processingAttempts: numberOrNull(uploaded.processingAttempts),
                    retryAvailable: Boolean(uploaded.retryUrl),
                    retryUrl: uploaded.retryUrl,
                  }
              : d
          ));
          saveState(nextDocs, assets, complianceGaps, certificates);
          return nextDocs;
        });
      }
    } catch {
      // Keep the local ingestion simulator below for offline demos.
    }

    // Simulate Background Ingestion Pipeline
    const statuses: Array<Document['status']> = [
      'EXTRACTING_TEXT',
      'OCR_RUNNING',
      'CLASSIFYING',
      'CHUNKING',
      'EXTRACTING_ENTITIES',
      'GENERATING_EMBEDDINGS',
      'BUILDING_GRAPH',
      'COMPLETED'
    ];

    let currentStatusIndex = 0;

    const interval = setInterval(() => {
      if (currentStatusIndex < statuses.length) {
        const nextStatus = statuses[currentStatusIndex];
        setDocuments(prevDocs => {
          const nextDocs = prevDocs.map(d => {
            if (d.id === newDocId) {
              const updatedDoc = { 
                ...d, 
                status: nextStatus,
                ocrConfidence: nextStatus === 'COMPLETED' ? 0.95 : d.ocrConfidence,
                classificationConfidence: nextStatus === 'COMPLETED' ? 0.92 : d.classificationConfidence
              };
              return updatedDoc;
            }
            if (d.id === activeDocId) {
              return {
                ...d,
                status: nextStatus,
                ocrConfidence: nextStatus === 'COMPLETED' ? 0.95 : d.ocrConfidence,
                classificationConfidence: nextStatus === 'COMPLETED' ? 0.92 : d.classificationConfidence
              };
            }
            return d;
          });
          saveState(nextDocs, assets, complianceGaps, certificates);
          return nextDocs;
        });
        currentStatusIndex++;
      } else {
        clearInterval(interval);
        
        // After completion, if the document mentions an asset tag like P-101, link it
        // Check for tags in document title
        const detectedTag = detectedAssetTags[0];
        if (detectedTag) {
          setAssets(prevAssets => {
            const nextAssets = prevAssets.map(a => {
              if (a.assetTag === detectedTag && !a.documents.includes(activeDocId)) {
                return { ...a, documents: [...a.documents, activeDocId] };
              }
              return a;
            });
            saveState(documents, nextAssets, complianceGaps, certificates);
            return nextAssets;
          });
        }
      }
    }, 1200);
  };

  const refreshDocumentStatus = async (documentId: string) => {
    if (documentId.startsWith('doc_')) return;

    const res = await apiFetch(`/api/documents/${encodeURIComponent(documentId)}/status`);
    if (!res.ok) {
      throw new Error(await readApiError(res, 'Unable to refresh document status'));
    }

    const status = await res.json();
    setDocuments(prevDocs => {
      const nextDocs = prevDocs.map(doc => (
        doc.id === documentId
          ? {
              ...doc,
              status: normalizeDocumentStatus(status.status || doc.status),
              processingJobId: status.processingJobId,
              processingStatus: status.processingStatus,
              processingProgress: numberOrNull(status.processingProgress),
              processingAttempts: numberOrNull(status.processingAttempts),
              retryAvailable: status.retryAvailable,
              retryUrl: status.retryUrl,
              processingError: status.errorMessage || status.processingError,
              jobUpdatedAt: status.jobUpdatedAt || status.updatedAt,
            }
          : doc
      ));
      saveState(nextDocs, assets, complianceGaps, certificates);
      return nextDocs;
    });
  };

  const retryDocumentProcessing = async (documentId: string) => {
    const existing = documents.find(doc => doc.id === documentId);
    if (!existing) {
      throw new Error('Document not found.');
    }

    if (documentId.startsWith('doc_')) {
      setDocuments(prevDocs => {
        const nextDocs = prevDocs.map(doc => (
          doc.id === documentId
            ? {
                ...doc,
                status: 'UPLOADED' as Document['status'],
                processingStatus: 'QUEUED',
                processingProgress: 0,
                processingAttempts: (numberOrNull((doc as Document & DocumentProcessingFields).processingAttempts) || 0) + 1,
                retryAvailable: false,
                processingError: undefined,
              }
            : doc
        ));
        saveState(nextDocs, assets, complianceGaps, certificates);
        return nextDocs;
      });
      return;
    }

    const res = await apiFetch(`/api/documents/${encodeURIComponent(documentId)}/retry-processing`, {
      method: 'POST',
    });
    const body = await res.json().catch(() => ({}));
    if (!res.ok) {
      throw new Error(body.error || body.message || await readApiError(res, 'Unable to retry processing'));
    }

    setDocuments(prevDocs => {
      const nextDocs = prevDocs.map(doc => (
        doc.id === documentId
          ? {
              ...doc,
              status: normalizeDocumentStatus(body.status === 'QUEUED' ? 'UPLOADED' : body.status || doc.status),
              processingStatus: body.status || 'QUEUED',
              processingProgress: numberOrNull(body.processingProgress) ?? 0,
              processingAttempts: numberOrNull(body.processingAttempts) ?? ((numberOrNull((doc as Document & DocumentProcessingFields).processingAttempts) || 0) + 1),
              retryAvailable: Boolean(body.retryUrl),
              retryUrl: body.retryUrl,
              processingError: body.status === 'FAILED' ? body.message : undefined,
              jobUpdatedAt: new Date().toISOString(),
            }
          : doc
      ));
      saveState(nextDocs, assets, complianceGaps, certificates);
      return nextDocs;
    });
  };

  const archiveDocument = async (documentId: string) => {
    const isLocalDemoDocument = documentId.startsWith('doc_');

    try {
      const res = await apiFetch(`/api/documents/${encodeURIComponent(documentId)}`, {
        method: 'DELETE',
      });

      if (!res.ok && !isLocalDemoDocument) {
        throw new Error(await readApiError(res, 'Unable to archive document'));
      }
    } catch (error) {
      if (!isLocalDemoDocument) {
        throw error;
      }
    }

    setDocuments(prevDocs => {
      const nextDocs = prevDocs.filter(doc => doc.id !== documentId);
      saveState(nextDocs, assets, complianceGaps, certificates);
      return nextDocs;
    });
    setAssets(prevAssets => {
      const nextAssets = prevAssets.map(asset => ({
        ...asset,
        documents: asset.documents.filter(id => id !== documentId),
      }));
      saveState(documents.filter(doc => doc.id !== documentId), nextAssets, complianceGaps, certificates);
      return nextAssets;
    });
  };

  const resolveGap = (gapId: string) => {
    const updatedGaps = complianceGaps.map(g => {
      if (g.id === gapId) {
        return { ...g, status: 'Closed' as const };
      }
      return g;
    });
    setComplianceGaps(updatedGaps);
    saveState(documents, assets, updatedGaps, certificates);
  };

  const addFailureEvent = (assetTag: string, description: string, severity: 'Low' | 'Medium' | 'High' | 'Critical') => {
    const newFail = {
      id: `fail_${Date.now()}`,
      date: new Date().toISOString().split('T')[0],
      description,
      severity,
      status: 'Open' as const,
      workOrder: `WO-${Math.floor(100 + Math.random() * 900)}`,
      maintenanceAction: 'Investigation open / awaiting diagnosis report'
    };

    const updatedAssets = assets.map(a => {
      if (a.assetTag === assetTag) {
        const nextFailures = [newFail, ...a.failures];
        // recalculate risk score slightly up
        const nextRisk = Math.min(100, a.riskScore + (severity === 'Critical' ? 15 : severity === 'High' ? 10 : 5));
        return { ...a, failures: nextFailures, riskScore: nextRisk };
      }
      return a;
    });

    setAssets(updatedAssets);
    saveState(documents, updatedAssets, complianceGaps, certificates);
  };

  return (
    <DataContext.Provider value={{ 
      documents, 
      assets, 
      complianceGaps, 
      certificates, 
      uploadDocument, 
      uploadDocumentVersion,
      retryDocumentProcessing,
      refreshDocumentStatus,
      archiveDocument,
      resolveGap,
      addFailureEvent
    }}>
      {children}
    </DataContext.Provider>
  );
};

function safelyHydrate<T>(raw: string | null, setter: React.Dispatch<React.SetStateAction<T>>) {
  if (!raw) return;

  try {
    setter(JSON.parse(raw));
  } catch {
    // Keep seeded demo data if a previous localStorage value was corrupted.
  }
}

function bumpVersionLabel(version: string) {
  const match = /^v(\d+)$/i.exec(version.trim());
  if (!match) return 'v2';
  return `v${Number(match[1]) + 1}`;
}

function numberOrNull(value: unknown) {
  if (value === null || value === undefined || value === '') return null;
  const number = Number(value);
  return Number.isFinite(number) ? number : null;
}

function isActiveProcessingStatus(status: Document['status']) {
  return !['COMPLETED', 'FAILED', 'PARTIAL_SUCCESS'].includes(status);
}

function isRetryCandidate(document: Document & DocumentProcessingFields) {
  return Boolean(document.retryAvailable || document.retryUrl || document.status === 'FAILED' || document.processingStatus === 'FAILED');
}

export const useData = () => {
  const context = useContext(DataContext);
  if (context === undefined) {
    throw new Error('useData must be used within a DataProvider');
  }
  return context;
};
