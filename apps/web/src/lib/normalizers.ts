import { Asset, ComplianceGap, Document } from './mockData';

const DOCUMENT_TYPE_LABELS: Record<string, string> = {
  OEM_MANUAL: 'OEM Manual',
  MANUAL: 'OEM Manual',
  SOP: 'SOP',
  MAINTENANCE_LOG: 'Maintenance Work Order',
  MAINTENANCE_WORK_ORDER: 'Maintenance Work Order',
  WORK_ORDER: 'Maintenance Work Order',
  INSPECTION_REPORT: 'Inspection Report',
  INCIDENT_REPORT: 'Incident Report',
  AUDIT_REPORT: 'Audit Report',
  SAFETY_PROCEDURE: 'Safety Procedure',
  PID: 'P&ID',
  P_AND_ID: 'P&ID',
  COMPLIANCE_DOCUMENT: 'Compliance Document',
  TRAINING_DOCUMENT: 'Training Document',
  UNKNOWN: 'Unknown',
};

export function normalizeDocumentType(value: unknown): string {
  const raw = String(value || 'Unknown').trim();
  if (!raw) return 'Unknown';

  const key = raw.toUpperCase().replace(/[\s-]+/g, '_').replace(/&/g, 'AND');
  if (DOCUMENT_TYPE_LABELS[key]) return DOCUMENT_TYPE_LABELS[key];

  if (!raw.includes('_')) return raw;

  return raw
    .toLowerCase()
    .split('_')
    .filter(Boolean)
    .map(part => part.charAt(0).toUpperCase() + part.slice(1))
    .join(' ');
}

export function normalizeDocumentStatus(value: unknown): Document['status'] {
  const status = String(value || 'UPLOADED').toUpperCase() as Document['status'];
  const knownStatuses: Document['status'][] = [
    'UPLOADED',
    'EXTRACTING_TEXT',
    'OCR_RUNNING',
    'CLASSIFYING',
    'CHUNKING',
    'EXTRACTING_ENTITIES',
    'GENERATING_EMBEDDINGS',
    'BUILDING_GRAPH',
    'COMPLETED',
    'FAILED',
    'PARTIAL_SUCCESS',
  ];
  return knownStatuses.includes(status) ? status : 'UPLOADED';
}

export function isDocumentTerminalStatus(status: Document['status']) {
  return status === 'COMPLETED' || status === 'FAILED' || status === 'PARTIAL_SUCCESS';
}

export function formatStatusLabel(status: string) {
  return status
    .replace(/_/g, ' ')
    .toLowerCase()
    .replace(/\b\w/g, char => char.toUpperCase());
}

export function normalizeSeverity(value: unknown): ComplianceGap['severity'] {
  const normalized = String(value || 'Medium').toLowerCase();
  if (normalized === 'critical') return 'Critical';
  if (normalized === 'high') return 'High';
  if (normalized === 'low') return 'Low';
  return 'Medium';
}

export function normalizeGapStatus(value: unknown): ComplianceGap['status'] {
  return String(value || '').toUpperCase() === 'CLOSED' ? 'Closed' : 'Open';
}

export function normalizeCriticality(value: unknown): Asset['criticality'] {
  const normalized = String(value || 'Medium').toLowerCase();
  if (normalized === 'critical') return 'Critical';
  if (normalized === 'high') return 'High';
  if (normalized === 'low') return 'Low';
  return 'Medium';
}
