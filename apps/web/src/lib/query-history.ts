import { CopilotResponse } from './mockData';

const QUERY_HISTORY_KEY = 'plantbrain_query_answers';
const MAX_HISTORY_ITEMS = 50;

export interface QueryAnswerRecord {
  id: string;
  query: string;
  answer: string;
  confidence: number;
  assetFilter?: string;
  citations: string[];
  missingInfo: string[];
  createdAt: string;
}

export function getQueryAnswerHistory(): QueryAnswerRecord[] {
  if (typeof window === 'undefined') return [];

  try {
    const raw = localStorage.getItem(QUERY_HISTORY_KEY);
    if (!raw) return [];
    const parsed = JSON.parse(raw);
    return Array.isArray(parsed) ? parsed : [];
  } catch {
    return [];
  }
}

export function recordQueryAnswer(query: string, response: CopilotResponse, assetFilter?: string) {
  if (typeof window === 'undefined') return;

  const nextRecord: QueryAnswerRecord = {
    id: `query_${Date.now()}`,
    query,
    answer: response.answer,
    confidence: response.confidence,
    assetFilter: assetFilter || undefined,
    citations: response.citations.map(citation => `${citation.documentTitle} p.${citation.page}`),
    missingInfo: response.missingInfo,
    createdAt: new Date().toISOString(),
  };

  const nextHistory = [nextRecord, ...getQueryAnswerHistory()].slice(0, MAX_HISTORY_ITEMS);
  localStorage.setItem(QUERY_HISTORY_KEY, JSON.stringify(nextHistory));
}
