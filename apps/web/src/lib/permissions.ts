import { DocumentAccessFields, DocumentAccessLevel, PlantBrainSession } from './api';

export type RoleKey = 'admin' | 'plant_manager' | 'engineer' | 'technician' | 'compliance_officer' | 'viewer';

export type PermissionAction =
  | 'view_dashboard'
  | 'view_documents'
  | 'upload_document'
  | 'archive_document'
  | 'download_restricted_document'
  | 'view_assets'
  | 'log_failure'
  | 'query_copilot'
  | 'generate_rca'
  | 'view_compliance'
  | 'run_compliance_scan'
  | 'resolve_compliance_gap'
  | 'view_graph'
  | 'export_reports'
  | 'manage_workspace_settings';

export const ROLE_DEFINITIONS: Array<{
  key: RoleKey;
  label: string;
  scope: string;
}> = [
  { key: 'admin', label: 'Admin', scope: 'Manage users, documents, settings, and all operational workflows' },
  { key: 'plant_manager', label: 'Plant Manager', scope: 'View dashboard, reports, risks, RCA, and compliance posture' },
  { key: 'engineer', label: 'Engineer', scope: 'Upload documents, query, manage assets, run RCA, and archive documents' },
  { key: 'technician', label: 'Technician', scope: 'Mobile SOP lookup, asset viewing, Copilot, and field event logging' },
  { key: 'compliance_officer', label: 'Compliance Officer', scope: 'Review compliance gaps, scan evidence, and export audit reports' },
  { key: 'viewer', label: 'Viewer', scope: 'Read-only operational access' },
];

const ACTION_PERMISSIONS: Record<PermissionAction, RoleKey[]> = {
  view_dashboard: ['admin', 'plant_manager', 'engineer', 'technician', 'compliance_officer', 'viewer'],
  view_documents: ['admin', 'plant_manager', 'engineer', 'technician', 'compliance_officer', 'viewer'],
  upload_document: ['admin', 'plant_manager', 'engineer', 'technician', 'compliance_officer', 'viewer'],
  archive_document: ['admin', 'engineer'],
  download_restricted_document: ['admin', 'plant_manager', 'engineer', 'compliance_officer'],
  view_assets: ['admin', 'plant_manager', 'engineer', 'technician', 'compliance_officer', 'viewer'],
  log_failure: ['admin', 'engineer', 'technician'],
  query_copilot: ['admin', 'plant_manager', 'engineer', 'technician', 'compliance_officer', 'viewer'],
  generate_rca: ['admin', 'plant_manager', 'engineer'],
  view_compliance: ['admin', 'plant_manager', 'engineer', 'compliance_officer'],
  run_compliance_scan: ['admin', 'plant_manager', 'compliance_officer'],
  resolve_compliance_gap: ['admin', 'plant_manager', 'compliance_officer'],
  view_graph: ['admin', 'plant_manager', 'engineer', 'technician', 'compliance_officer', 'viewer'],
  export_reports: ['admin', 'plant_manager', 'engineer', 'compliance_officer'],
  manage_workspace_settings: ['admin'],
};

const DOCUMENT_SOURCE_ROLES: Record<DocumentAccessLevel, RoleKey[]> = {
  public: ACTION_PERMISSIONS.view_documents,
  internal: ACTION_PERMISSIONS.view_documents,
  restricted: ACTION_PERMISSIONS.download_restricted_document,
  confidential: ['admin', 'compliance_officer'],
};

const ROUTE_ACTIONS: Array<{ prefix: string; action: PermissionAction }> = [
  { prefix: '/admin', action: 'manage_workspace_settings' },
  { prefix: '/reports', action: 'export_reports' },
  { prefix: '/compliance', action: 'view_compliance' },
  { prefix: '/rca', action: 'generate_rca' },
  { prefix: '/copilot', action: 'query_copilot' },
  { prefix: '/documents', action: 'view_documents' },
  { prefix: '/assets', action: 'view_assets' },
  { prefix: '/graph', action: 'view_graph' },
  { prefix: '/', action: 'view_dashboard' },
];

export function normalizeRole(role?: string | null): RoleKey {
  const normalized = String(role || '')
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '_')
    .replace(/^_+|_+$/g, '');

  if (normalized === 'admin' || normalized === 'administrator') return 'admin';
  if (normalized === 'plant_manager' || normalized === 'manager') return 'plant_manager';
  if (
    normalized === 'engineer'
    || normalized === 'maintenance_engineer'
    || normalized === 'reliability_engineer'
    || normalized === 'plant_engineer'
  ) return 'engineer';
  if (normalized === 'field_technician' || normalized === 'technician' || normalized === 'plant_operator' || normalized === 'operator') return 'technician';
  if (normalized === 'compliance_officer' || normalized === 'compliance') return 'compliance_officer';
  if (normalized === 'viewer' || normalized === 'read_only' || normalized === 'readonly') return 'viewer';

  return 'viewer';
}

export function getRoleLabel(role?: string | RoleKey | null) {
  const key = normalizeRole(role);
  return ROLE_DEFINITIONS.find(definition => definition.key === key)?.label || 'Viewer';
}

export function canRunAction(role: string | RoleKey | null | undefined, action: PermissionAction) {
  return ACTION_PERMISSIONS[action].includes(normalizeRole(role));
}

export function getDeniedMessage(role: string | RoleKey | null | undefined, action: PermissionAction) {
  const allowedRoles = ACTION_PERMISSIONS[action].map(getRoleLabel).join(', ');
  return `${getRoleLabel(role)} access does not include this action. Allowed roles: ${allowedRoles}.`;
}

export function getRouteAction(pathname: string): PermissionAction {
  const route = ROUTE_ACTIONS.find(item => pathname === item.prefix || (item.prefix !== '/' && pathname.startsWith(item.prefix)));
  return route?.action || 'view_dashboard';
}

export function canAccessPath(role: string | RoleKey | null | undefined, pathname: string) {
  return canRunAction(role, getRouteAction(pathname));
}

export function getSessionRole(session: PlantBrainSession | null | undefined) {
  return normalizeRole(session?.role);
}

export function getRoleCapabilities(role: string | RoleKey | null | undefined) {
  return (Object.keys(ACTION_PERMISSIONS) as PermissionAction[]).filter(action => canRunAction(role, action));
}

export function isRestrictedDocumentType(documentType?: string | null) {
  const normalized = String(documentType || '').toLowerCase();
  return normalized.includes('compliance') || normalized.includes('audit') || normalized.includes('incident') || normalized.includes('safety');
}

export function getDocumentAccessInfo(document?: (DocumentAccessFields & { documentType?: string | null }) | null) {
  const explicitAccessLevel = normalizeAccessLevel(document?.accessLevel);
  const sensitivity = normalizeSensitivity(document?.sensitivity);
  const classificationRestricted = isRestrictedDocumentType(document?.documentType);
  const elevatedSensitivity = sensitivity === 'sensitive' || sensitivity === 'confidential' || sensitivity === 'safety_critical';
  const accessLevel = explicitAccessLevel || (elevatedSensitivity || classificationRestricted ? 'restricted' : 'internal');
  const allowedRoles = normalizeAllowedRoles(document?.allowedRoles, DOCUMENT_SOURCE_ROLES[accessLevel]);
  const sourceRestricted = Boolean(document?.sourceRestricted) || accessLevel === 'restricted' || accessLevel === 'confidential' || allowedRoles.length < ACTION_PERMISSIONS.view_documents.length;

  return {
    accessLevel,
    label: getAccessLevelLabel(accessLevel),
    sensitivity,
    sensitivityLabel: getSensitivityLabel(sensitivity),
    allowedRoles,
    allowedRoleLabels: allowedRoles.map(getRoleLabel),
    sourceRestricted,
    classificationRestricted,
  };
}

export function canDownloadDocumentSource(role: string | RoleKey | null | undefined, document?: (DocumentAccessFields & { documentType?: string | null }) | null) {
  const access = getDocumentAccessInfo(document);
  if (document?.sourceDownloadAllowed === false) return false;
  if (document?.sourceDownloadAllowed === true) return true;
  return access.allowedRoles.includes(normalizeRole(role));
}

export function isDocumentSourceRestricted(document?: (DocumentAccessFields & { documentType?: string | null }) | null) {
  return getDocumentAccessInfo(document).sourceRestricted;
}

export function getDocumentSourceDeniedMessage(role: string | RoleKey | null | undefined, document?: (DocumentAccessFields & { documentType?: string | null }) | null) {
  const access = getDocumentAccessInfo(document);
  return `${getRoleLabel(role)} access can view metadata for this ${access.label.toLowerCase()} document, but source evidence downloads are limited to: ${access.allowedRoleLabels.join(', ')}.`;
}

export function getRoleDocumentAccessSummary(role: string | RoleKey | null | undefined) {
  const roleKey = normalizeRole(role);
  const levels = (Object.keys(DOCUMENT_SOURCE_ROLES) as DocumentAccessLevel[])
    .filter(level => DOCUMENT_SOURCE_ROLES[level].includes(roleKey))
    .map(getAccessLevelLabel);

  return levels.length > 0 ? levels.join(', ') : 'Metadata only';
}

function normalizeAccessLevel(value?: string | null): DocumentAccessLevel | null {
  const normalized = String(value || '').trim().toLowerCase().replace(/[^a-z0-9]+/g, '_');
  if (normalized === 'public') return 'public';
  if (normalized === 'internal') return 'internal';
  if (normalized === 'restricted') return 'restricted';
  if (normalized === 'confidential') return 'confidential';
  return null;
}

function normalizeSensitivity(value?: string | null) {
  const normalized = String(value || '').trim().toLowerCase().replace(/[^a-z0-9]+/g, '_');
  if (normalized === 'standard' || normalized === 'normal' || normalized === 'low' || normalized === 'medium') return 'standard';
  if (normalized === 'sensitive' || normalized === 'restricted' || normalized === 'high' || normalized === 'regulated') return 'sensitive';
  if (normalized === 'confidential') return 'confidential';
  if (normalized === 'safety_critical' || normalized === 'safety' || normalized === 'critical') return 'safety_critical';
  return 'standard';
}

function normalizeAllowedRoles(roles: string[] | null | undefined, fallback: RoleKey[]) {
  if (!roles || roles.length === 0) return fallback;

  const normalized = roles
    .map(parseAllowedRole)
    .filter((role): role is RoleKey => Boolean(role));

  return normalized.length > 0 ? Array.from(new Set(normalized)) : fallback;
}

function parseAllowedRole(role: string): RoleKey | null {
  const normalized = String(role || '')
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '_')
    .replace(/^_+|_+$/g, '');

  if (!normalized) return null;
  if (normalized === 'admin' || normalized === 'administrator') return 'admin';
  if (normalized === 'plant_manager' || normalized === 'manager') return 'plant_manager';
  if (
    normalized === 'engineer'
    || normalized === 'maintenance_engineer'
    || normalized === 'reliability_engineer'
    || normalized === 'plant_engineer'
  ) return 'engineer';
  if (normalized === 'field_technician' || normalized === 'technician' || normalized === 'plant_operator' || normalized === 'operator') return 'technician';
  if (normalized === 'compliance_officer' || normalized === 'compliance') return 'compliance_officer';
  if (normalized === 'viewer' || normalized === 'read_only' || normalized === 'readonly') return 'viewer';

  return null;
}

function getAccessLevelLabel(accessLevel: DocumentAccessLevel) {
  if (accessLevel === 'public') return 'Public';
  if (accessLevel === 'internal') return 'Internal';
  if (accessLevel === 'restricted') return 'Restricted';
  return 'Confidential';
}

function getSensitivityLabel(sensitivity: ReturnType<typeof normalizeSensitivity>) {
  if (sensitivity === 'standard') return 'Standard';
  if (sensitivity === 'sensitive') return 'Sensitive';
  if (sensitivity === 'confidential') return 'Confidential';
  return 'Safety critical';
}
