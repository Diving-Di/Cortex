import { http } from './http';
import axios from 'axios';

export interface XHSAuthorization {
  id: number;
  status: 'pending' | 'authorized' | 'expired' | 'revoked' | 'failed';
  account_display_name?: string | null;
  authorized_at?: string | null;
  last_verified_at?: string | null;
  expires_at?: string | null;
  failure_code?: string | null;
  requires_reauthorization?: boolean;
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

export async function getXHSAuthorization() {
  try {
    const response = await http.get<XHSAuthorization>('/api/v1/research/xhs/authorization', {});
    return response.data;
  } catch (error) {
    if (axios.isAxiosError(error) && error.response?.status === 404) return null;
    throw error;
  }
}

export async function startXHSAuthorization() {
  const response = await http.post<XHSAuthAttempt>(
    '/api/v1/research/xhs/authorizations',
    undefined,
    {},
  );
  return response.data;
}

export async function getXHSAuthAttempt(id: string) {
  const response = await http.get<XHSAuthAttempt>(`/api/v1/research/xhs/authorizations/${id}`, {});
  return response.data;
}

export async function loadXHSAuthQR(id: string) {
  const response = await http.get<Blob>(`/api/v1/research/xhs/authorizations/${id}/qr`, {
    responseType: 'blob',
  });
  return URL.createObjectURL(response.data);
}

export async function cancelXHSAuthorization(id: string) {
  await http.post(`/api/v1/research/xhs/authorizations/${id}/cancel`, undefined, {});
}

export async function verifyXHSAuthorization() {
  const response = await http.post<XHSAuthorization>(
    '/api/v1/research/xhs/authorization/verify',
    undefined,
    {},
  );
  return response.data;
}

export async function revokeXHSAuthorization() {
  await http.delete('/api/v1/research/xhs/authorization', {});
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
  like_count: number;
  collect_count: number;
  comment_count: number;
  raw_content: string;
  formatted_content: string;
  parse_strategy: string;
  content_completeness: number;
  ocr_contribution_chars: number;
  format_status: 'deterministic' | 'ai_formatted' | 'ai_unavailable' | 'ai_failed';
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

export async function createResearchJob(value: {
  mode: 'keyword' | 'urls';
  keywords?: string[];
  urls?: string[];
  target_count: number;
  search_sort?: 'general' | 'time_descending' | 'popularity_descending';
  idempotency_key: string;
}) {
  const response = await http.post<ResearchJob>('/api/v1/research/jobs', value, {});
  return response.data;
}

export async function listResearchJobs(page = 1) {
  const response = await http.get<{ items: ResearchJob[]; total: number }>(
    '/api/v1/research/jobs',
    {
      params: { limit: 20, offset: (page - 1) * 20 },
    },
  );
  return { ...response.data, items: response.data.items || [] };
}

export async function cancelResearchJob(id: number) {
  await http.post(`/api/v1/research/jobs/${id}/cancel`, undefined, {});
}

export async function retryResearchJob(id: number) {
  await http.post(`/api/v1/research/jobs/${id}/retry`, undefined, {});
}

export async function listResearchSources(
  query: { jobId?: number; status?: string; search?: string; sort?: string; page?: number } = {},
) {
  const response = await http.get<{ items: ResearchSource[]; total: number }>(
    '/api/v1/research/sources',
    {
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
  return { ...response.data, items: response.data.items || [] };
}

export async function getResearchSource(id: number) {
  const response = await http.get<ResearchSource>(`/api/v1/research/sources/${id}`, {});
  return response.data;
}

export async function updateResearchDraft(
  sourceId: number,
  value: Pick<ResearchDraft, 'summary' | 'key_points' | 'category' | 'suggested_tags' | 'version'>,
) {
  const response = await http.patch<ResearchDraft>(
    `/api/v1/research/sources/${sourceId}/draft`,
    value,
    {},
  );
  return response.data;
}

export async function ignoreResearchSource(id: number) {
  await http.post(`/api/v1/research/sources/${id}/ignore`, undefined, {});
}

export async function recollectResearchSource(id: number) {
  await http.post(`/api/v1/research/sources/${id}/recollect`, undefined, {});
}

export async function deleteResearchSource(id: number) {
  await http.delete(`/api/v1/research/sources/${id}`, {});
}

export async function batchSaveResearchSources(ids: number[]) {
  await http.post('/api/v1/research/sources/batch-save', { ids }, {});
}

export async function batchIgnoreResearchSources(ids: number[]) {
  await http.post('/api/v1/research/sources/batch-ignore', { ids }, {});
}

export async function loadResearchAsset(id: number) {
  const response = await http.get<Blob>(`/api/v1/research/assets/${id}`, {
    responseType: 'blob',
  });
  return URL.createObjectURL(response.data);
}
