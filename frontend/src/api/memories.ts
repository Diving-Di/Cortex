import { authHeaders, http } from './http';
export type MemoryCategory = 'fact' | 'preference' | 'goal' | 'habit' | 'milestone';
export interface GrowthMemory {
  id: number;
  category: MemoryCategory;
  content: string;
  importance: number;
  source_type: string;
  creation_mode: string;
  version: number;
  created_at: string;
  updated_at: string;
}
export interface MemorySettings {
  suggestion_enabled: boolean;
  allowed_categories: MemoryCategory[];
  minimum_importance: number;
  excluded_note_types: string[];
  excluded_tag_ids: number[];
  retention_days?: number | null;
}
export async function listMemories(token: string, search = '', category = '') {
  return (
    await http.get<{ items: GrowthMemory[]; total: number }>('/api/v1/growth-memories', {
      headers: authHeaders(token),
      params: { search, category },
    })
  ).data;
}
export async function createMemory(token: string, value: Partial<GrowthMemory>) {
  return (
    await http.post<GrowthMemory>('/api/v1/growth-memories', value, { headers: authHeaders(token) })
  ).data;
}
export async function updateMemory(token: string, value: GrowthMemory) {
  return (
    await http.patch<GrowthMemory>(`/api/v1/growth-memories/${value.id}`, value, {
      headers: authHeaders(token),
    })
  ).data;
}
export async function deleteMemory(token: string, id: number) {
  await http.delete(`/api/v1/growth-memories/${id}`, { headers: authHeaders(token) });
}
export async function getMemorySettings(token: string) {
  return (
    await http.get<MemorySettings>('/api/v1/settings/memories', { headers: authHeaders(token) })
  ).data;
}
export async function saveMemorySettings(token: string, value: MemorySettings) {
  return (
    await http.put<MemorySettings>('/api/v1/settings/memories', value, {
      headers: authHeaders(token),
    })
  ).data;
}
export interface MemoryDraftItem {
  category: MemoryCategory;
  content: string;
  importance: number;
  source_type: 'note' | 'conversation' | 'message';
  source_id: number;
}
export interface MemoryDraft {
  draft_id: string;
  items: MemoryDraftItem[];
  status: string;
  expires_at: string;
}
export async function createMemoryDraft(token: string, source_type: string, source_id: number) {
  return (
    await http.post<MemoryDraft>(
      '/api/v1/growth-memory-drafts',
      { source_type, source_id },
      { headers: authHeaders(token) },
    )
  ).data;
}
export async function confirmMemoryDraft(token: string, draft: MemoryDraft) {
  return (
    await http.post(
      `/api/v1/growth-memory-drafts/${draft.draft_id}/confirm`,
      { items: draft.items },
      { headers: authHeaders(token) },
    )
  ).data;
}
export async function rejectMemoryDraft(token: string, id: string) {
  await http.post(`/api/v1/growth-memory-drafts/${id}/reject`, undefined, {
    headers: authHeaders(token),
  });
}
