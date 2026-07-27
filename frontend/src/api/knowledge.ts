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

export interface KnowledgeDocumentQuery {
  collectionId?: number;
  search?: string;
  status?: KnowledgeDocument['status'];
  page?: number;
  pageSize?: number;
}

export async function listKnowledgeDocuments(token: string, query: KnowledgeDocumentQuery = {}) {
  const { collectionId, search, status, page = 1, pageSize = 20 } = query;
  const response = await http.get<{ items: KnowledgeDocument[]; total: number }>(
    '/api/v1/knowledge/documents',
    {
      headers: authHeaders(token),
      params: {
        limit: pageSize,
        offset: (page - 1) * pageSize,
        ...(collectionId ? { collection_id: collectionId } : {}),
        ...(search ? { search } : {}),
        ...(status ? { status } : {}),
      },
    },
  );
  return response.data;
}

export async function getKnowledgeDocument(token: string, id: number) {
  const response = await http.get<KnowledgeDocument>(`/api/v1/knowledge/documents/${id}`, {
    headers: authHeaders(token),
  });
  return response.data;
}

export async function getKnowledgePreview(token: string, id: number) {
  const response = await http.get<{ preview: string }>(
    `/api/v1/knowledge/documents/${id}/preview`,
    { headers: authHeaders(token) },
  );
  return response.data.preview;
}

export async function reindexKnowledgeDocument(token: string, id: number) {
  await http.post(`/api/v1/knowledge/documents/${id}/reindex`, undefined, {
    headers: authHeaders(token),
  });
}

export async function downloadKnowledgeDocument(token: string, document: KnowledgeDocument) {
  const response = await http.get<Blob>(`/api/v1/knowledge/documents/${document.id}/download`, {
    headers: authHeaders(token),
    responseType: 'blob',
  });
  const url = URL.createObjectURL(response.data);
  const anchor = window.document.createElement('a');
  anchor.href = url;
  anchor.download = document.original_name;
  anchor.click();
  URL.revokeObjectURL(url);
}

export async function deleteKnowledgeCollection(token: string, id: number) {
  await http.delete(`/api/v1/knowledge/collections/${id}`, { headers: authHeaders(token) });
}

export interface Conversation {
  id: number;
  title: string;
  source_scope: 'knowledge' | 'growth' | 'all';
  created_at: string;
  updated_at: string;
  messages?: Array<{ id: number; role: 'user' | 'assistant'; content: string; created_at: string }>;
}

export async function listConversations(token: string) {
  const response = await http.get<Conversation[]>('/api/v1/conversations', {
    headers: authHeaders(token),
  });
  return response.data || [];
}

export async function createConversation(token: string, sourceScope: Conversation['source_scope']) {
  const response = await http.post<Conversation>(
    '/api/v1/conversations',
    { title: '新对话', source_scope: sourceScope },
    { headers: authHeaders(token) },
  );
  return response.data;
}

export async function getConversation(token: string, id: number) {
  const response = await http.get<Conversation>(`/api/v1/conversations/${id}`, {
    headers: authHeaders(token),
  });
  return response.data;
}

export async function deleteConversation(token: string, id: number) {
  await http.delete(`/api/v1/conversations/${id}`, { headers: authHeaders(token) });
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
