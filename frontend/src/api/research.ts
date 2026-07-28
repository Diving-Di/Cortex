import { authHeaders, http } from './http';
import axios from 'axios';

export interface XHSAuthorization {
  id: number;
  status: 'pending' | 'authorized' | 'expired' | 'revoked' | 'failed';
  account_display_name?: string | null;
  authorized_at?: string | null;
  last_verified_at?: string | null;
  expires_at?: string | null;
  failure_code?: string | null;
  version: number;
}

export interface XHSAuthAttempt {
  id: string;
  authorization_id: number;
  status:
    | 'queued'
    | 'starting'
    | 'waiting_for_scan'
    | 'scanned'
    | 'verification_required'
    | 'authorized'
    | 'failed'
    | 'cancelled'
    | 'expired';
  failure_code?: string | null;
  expires_at: string;
  created_at: string;
  updated_at: string;
}

export async function getXHSAuthorization(token: string) {
  try {
    const response = await http.get<XHSAuthorization>('/api/v1/research/xhs/authorization', {
      headers: authHeaders(token),
    });
    return response.data;
  } catch (error) {
    if (axios.isAxiosError(error) && error.response?.status === 404) return null;
    throw error;
  }
}

export async function startXHSAuthorization(token: string) {
  const response = await http.post<XHSAuthAttempt>(
    '/api/v1/research/xhs/authorizations',
    undefined,
    { headers: authHeaders(token) },
  );
  return response.data;
}

export async function getXHSAuthAttempt(token: string, id: string) {
  const response = await http.get<XHSAuthAttempt>(`/api/v1/research/xhs/authorizations/${id}`, {
    headers: authHeaders(token),
  });
  return response.data;
}

export async function loadXHSAuthQR(token: string, id: string) {
  const response = await http.get<Blob>(`/api/v1/research/xhs/authorizations/${id}/qr`, {
    headers: authHeaders(token),
    responseType: 'blob',
  });
  return URL.createObjectURL(response.data);
}

export async function cancelXHSAuthorization(token: string, id: string) {
  await http.post(`/api/v1/research/xhs/authorizations/${id}/cancel`, undefined, {
    headers: authHeaders(token),
  });
}

export async function verifyXHSAuthorization(token: string) {
  const response = await http.post<XHSAuthorization>(
    '/api/v1/research/xhs/authorization/verify',
    undefined,
    { headers: authHeaders(token) },
  );
  return response.data;
}

export async function revokeXHSAuthorization(token: string) {
  await http.delete('/api/v1/research/xhs/authorization', { headers: authHeaders(token) });
}

export type ResearchJobStatus =
  | 'queued'
  | 'collecting'
  | 'extracting'
  | 'organizing'
  | 'reviewing'
  | 'completed'
  | 'failed'
  | 'cancelled';

export interface ResearchJob {
  id: number;
  mode: 'keyword' | 'urls';
  query_payload: { keywords?: string[]; urls?: string[] };
  target_count: number;
  target_collection_id?: number | null;
  status: ResearchJobStatus;
  found_count: number;
  collected_count: number;
  organized_count: number;
  failed_count: number;
  saved_count: number;
  attempt_count: number;
  max_attempts: number;
  last_error_code?: string | null;
  last_error_summary?: string | null;
  cancel_requested_at?: string | null;
  created_at: string;
  updated_at: string;
}

export interface ResearchDraft {
  id: number;
  summary: string;
  key_points: string[];
  category: string;
  suggested_tags: string[];
  edited_by_user: boolean;
  status: 'pending' | 'saved' | 'ignored';
  knowledge_document_id?: number | null;
  source_snapshot_hash: string;
  version: number;
  created_at: string;
  updated_at: string;
}

export interface ResearchAsset {
  id: number;
  position: number;
  mime_type: string;
  byte_size: number;
  sha256: string;
  ocr_status: 'pending' | 'processing' | 'ready' | 'failed' | 'unavailable';
  ocr_text: string;
  created_at: string;
}

export interface ResearchSource {
  id: number;
  job_id: number;
  source_url: string;
  normalized_url: string;
  title: string;
  author_display_name: string;
  published_at?: string | null;
  raw_content: string;
  public_tags: string[];
  status:
    | 'pending'
    | 'collecting'
    | 'organizing'
    | 'pending_review'
    | 'saved'
    | 'ignored'
    | 'failed';
  failure_code?: string | null;
  failure_summary?: string | null;
  collected_at?: string | null;
  version: number;
  created_at: string;
  updated_at: string;
  draft?: ResearchDraft;
  assets?: ResearchAsset[];
}

export async function createResearchJob(
  token: string,
  value: {
    mode: 'keyword' | 'urls';
    keywords?: string[];
    urls?: string[];
    target_count: number;
    target_collection_id?: number;
    idempotency_key: string;
  },
) {
  const response = await http.post<ResearchJob>('/api/v1/research/jobs', value, {
    headers: authHeaders(token),
  });
  return response.data;
}

export async function listResearchJobs(token: string, page = 1) {
  const response = await http.get<{ items: ResearchJob[]; total: number }>(
    '/api/v1/research/jobs',
    {
      headers: authHeaders(token),
      params: { limit: 20, offset: (page - 1) * 20 },
    },
  );
  return response.data;
}

export async function cancelResearchJob(token: string, id: number) {
  await http.post(`/api/v1/research/jobs/${id}/cancel`, undefined, { headers: authHeaders(token) });
}

export async function retryResearchJob(token: string, id: number) {
  await http.post(`/api/v1/research/jobs/${id}/retry`, undefined, { headers: authHeaders(token) });
}

export async function listResearchSources(
  token: string,
  query: { jobId?: number; status?: string; search?: string; sort?: string; page?: number } = {},
) {
  const response = await http.get<{ items: ResearchSource[]; total: number }>(
    '/api/v1/research/sources',
    {
      headers: authHeaders(token),
      params: {
        limit: 20,
        offset: ((query.page || 1) - 1) * 20,
        ...(query.jobId ? { job_id: query.jobId } : {}),
        ...(query.status ? { status: query.status } : {}),
        ...(query.search ? { search: query.search } : {}),
        ...(query.sort ? { sort: query.sort } : {}),
      },
    },
  );
  return response.data;
}

export async function getResearchSource(token: string, id: number) {
  const response = await http.get<ResearchSource>(`/api/v1/research/sources/${id}`, {
    headers: authHeaders(token),
  });
  return response.data;
}

export async function updateResearchDraft(
  token: string,
  sourceId: number,
  value: Pick<ResearchDraft, 'summary' | 'key_points' | 'category' | 'suggested_tags' | 'version'>,
) {
  const response = await http.patch<ResearchDraft>(
    `/api/v1/research/sources/${sourceId}/draft`,
    value,
    { headers: authHeaders(token) },
  );
  return response.data;
}

export async function saveResearchSource(token: string, id: number) {
  await http.post(`/api/v1/research/sources/${id}/save`, undefined, {
    headers: authHeaders(token),
  });
}

export async function ignoreResearchSource(token: string, id: number) {
  await http.post(`/api/v1/research/sources/${id}/ignore`, undefined, {
    headers: authHeaders(token),
  });
}

export async function recollectResearchSource(token: string, id: number) {
  await http.post(`/api/v1/research/sources/${id}/recollect`, undefined, {
    headers: authHeaders(token),
  });
}

export async function deleteResearchSource(token: string, id: number) {
  await http.delete(`/api/v1/research/sources/${id}`, { headers: authHeaders(token) });
}

export async function batchSaveResearchSources(token: string, ids: number[]) {
  await http.post('/api/v1/research/sources/batch-save', { ids }, { headers: authHeaders(token) });
}

export async function batchIgnoreResearchSources(token: string, ids: number[]) {
  await http.post(
    '/api/v1/research/sources/batch-ignore',
    { ids },
    { headers: authHeaders(token) },
  );
}

export async function loadResearchAsset(token: string, id: number) {
  const response = await http.get<Blob>(`/api/v1/research/assets/${id}`, {
    headers: authHeaders(token),
    responseType: 'blob',
  });
  return URL.createObjectURL(response.data);
}
