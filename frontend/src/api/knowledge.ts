import { http, authHeaders } from './http';

export type KnowledgeDocument = {
  id: string;
  upload_id?: string;
  collection_id?: string;
  SourceType: 'upload' | 'note';
  Title: string;
  Status: 'uploaded' | 'parsing' | 'indexing' | 'ready' | 'failed' | 'deleting';
  size_bytes: number;
  active_index_version: number;
  failure_code?: string;
  failure_summary?: string;
  CreatedAt: string;
  UpdatedAt: string;
};

export type KnowledgeList = {
  items: KnowledgeDocument[] | null;
  quota: {
    limit_bytes: number;
    used_bytes: number;
    reserved_bytes: number;
    remaining_bytes: number;
  };
};

export async function listKnowledge(token: string) {
  return (
    await http.get<KnowledgeList>('/api/v1/knowledge/documents', { headers: authHeaders(token) })
  ).data;
}
export async function uploadKnowledge(token: string, file: File) {
  const body = new FormData();
  body.append('file', file);
  return (await http.post('/api/v1/knowledge/uploads', body, { headers: authHeaders(token) })).data;
}
export async function deleteKnowledge(token: string, id: string) {
  await http.delete(`/api/v1/knowledge/documents/${id}`, { headers: authHeaders(token) });
}
export async function setNoteKnowledge(token: string, noteID: number, enabled: boolean) {
  return (
    await http.patch(
      `/api/v1/notes/${noteID}/knowledge`,
      { enabled },
      { headers: authHeaders(token) },
    )
  ).data;
}
