export const DEFAULT_DEV_TOKEN = 'dev-token';
const SESSION_KEY = 'plantbrain_session';
const TOKEN_KEY = 'plantbrain_api_token';
const DEFAULT_SESSION_MS = 8 * 60 * 60 * 1000;

export interface PlantBrainSession {
  name: string;
  role: string;
  email: string;
  token?: string;
  userId?: string;
  orgId?: string;
  plantId?: string;
  plantName?: string;
  organizationName?: string;
  expiresAt?: string;
}

export function getStoredSession(): PlantBrainSession | null {
  if (typeof window === 'undefined') return null;

  const raw = localStorage.getItem(SESSION_KEY);
  if (!raw) return null;

  try {
    const parsed = JSON.parse(raw) as Partial<PlantBrainSession>;
    if (!parsed.email || !parsed.name || !parsed.role) return null;
    if (isSessionExpired(parsed)) {
      clearStoredSession();
      return null;
    }
    return parsed as PlantBrainSession;
  } catch {
    return null;
  }
}

export function setStoredSession(session: PlantBrainSession) {
  if (typeof window === 'undefined') return;

  const sessionWithExpiry = {
    ...session,
    expiresAt: session.expiresAt || new Date(Date.now() + DEFAULT_SESSION_MS).toISOString(),
  };

  localStorage.setItem(SESSION_KEY, JSON.stringify(sessionWithExpiry));
  if (session.token) {
    localStorage.setItem(TOKEN_KEY, session.token);
  }
}

export function mergeStoredSession(update: Partial<PlantBrainSession>) {
  const current = getStoredSession();
  if (!current) return null;

  const nextSession = { ...current, ...update };
  setStoredSession(nextSession);
  return nextSession;
}

export function clearStoredSession() {
  if (typeof window === 'undefined') return;

  localStorage.removeItem(SESSION_KEY);
  localStorage.removeItem(TOKEN_KEY);
}

export function getPlantId() {
  return getStoredSession()?.plantId;
}

export function getApiToken(): string {
  if (typeof window === 'undefined') {
    return DEFAULT_DEV_TOKEN;
  }

  const directToken = localStorage.getItem(TOKEN_KEY);
  if (directToken) {
    return directToken;
  }

  return getStoredSession()?.token || DEFAULT_DEV_TOKEN;
}

export function authHeaders(extra?: HeadersInit): HeadersInit {
  const headers = new Headers(extra);
  headers.set('Authorization', `Bearer ${getApiToken()}`);
  return headers;
}

export async function apiFetch(path: string, init: RequestInit = {}) {
  const response = await fetch(path, {
    ...init,
    headers: authHeaders(init.headers),
  });

  if (response.status === 401 || response.status === 403) {
    clearStoredSession();
  }

  return response;
}

export function appendPlantQuery(path: string, plantId = getPlantId()) {
  if (!plantId) return path;

  const separator = path.includes('?') ? '&' : '?';
  return `${path}${separator}plantId=${encodeURIComponent(plantId)}`;
}

export function isSessionExpired(session: Partial<PlantBrainSession>) {
  if (!session.expiresAt) return false;
  const expiresAt = Date.parse(session.expiresAt);
  return Number.isFinite(expiresAt) && expiresAt <= Date.now();
}
