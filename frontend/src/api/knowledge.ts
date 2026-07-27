import { authHeaders, http } from './http';

export interface KnowledgeCollection {
  id: number;
  name: string;
  description?: string | null;
  version: number;
  created_at: string;
  updated_at: string;
}

export interface KnowledgeDocument {
  id: number;
  collection_id?: number | null;
  original_name: string;
  mime_type: string;
  extension: '.txt' | '.pdf' | '.docx';
  size: number;
  sha256: string;
  status: 'uploaded' | 'extracting' | 'indexing' | 'ready' | 'failed' | 'deleting';
  page_count?: number | null;
  character_count: number;
  parent_chunk_count: number;
  child_chunk_count: number;
  error_code?: string | null;
  error_message?: string | null;
  created_at: string;
  updated_at: string;
}

export async function listKnowledgeCollections(token: string) {
  const response = await http.get<KnowledgeCollection[]>('/api/v1/knowledge/collections', {
    headers: authHeaders(token),
  });
  return response.data || [];
}

export async function createKnowledgeCollection(
  token: string,
  value: { name: string; description?: string },
) {
  const response = await http.post<KnowledgeCollection>('/api/v1/knowledge/collections', value, {
    headers: authHeaders(token),
  });
  return response.data;
}

export async function listKnowledgeDocuments(token: string, collectionId?: number) {
  const response = await http.get<{ items: KnowledgeDocument[]; total: number }>(
    '/api/v1/knowledge/documents',
    {
      headers: authHeaders(token),
      params: { limit: 100, ...(collectionId ? { collection_id: collectionId } : {}) },
    },
  );
  return response.data;
}

export async function uploadKnowledgeDocument(token: string, file: File, collectionId?: number) {
  const form = new FormData();
  form.append('file', file);
  if (collectionId) form.append('collection_id', String(collectionId));
  const response = await http.post<KnowledgeDocument>('/api/v1/knowledge/documents', form, {
    headers: authHeaders(token),
  });
  return response.data;
}

export async function deleteKnowledgeDocument(token: string, id: number) {
  await http.delete(`/api/v1/knowledge/documents/${id}`, { headers: authHeaders(token) });
}
