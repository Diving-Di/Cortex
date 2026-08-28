import { http } from './http';
import { consumeSSE, errorFromResponse } from './client';

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
  last_index_failure_code?: string;
  index_job_status?: 'queued' | 'running' | 'success' | 'failed';
  index_stage?:
    | 'queued'
    | 'loading'
    | 'parsing'
    | 'embedding'
    | 'persisting'
    | 'completed'
    | 'failed';
  processed_chunks?: number;
  total_chunks?: number;
  CreatedAt: string;
  UpdatedAt: string;
};

export type KnowledgeSource = {
  citation: string;
  document_id?: string;
  note_id?: number;
  source_type: 'upload' | 'note';
  title: string;
  heading: string[];
  rank: number;
};

export type RetrievalProgress = {
  schema_version: 1;
  stage:
    | 'rewrite'
    | 'embedding'
    | 'retrieval'
    | 'rerank'
    | 'evidence_gate'
    | 'generation'
    | 'verification';
  status: 'started' | 'completed' | 'degraded';
  elapsed_ms: number;
  candidate_count?: number;
  qualified_count?: number;
  source_count?: number;
  rewritten?: boolean;
  planned?: boolean;
  subquery_count?: number;
};

export type KnowledgeStreamEvent =
  | { type: 'retrieval_progress'; data: RetrievalProgress }
  | { type: 'retrieval'; data: { count: number; items: KnowledgeSource[] } }
  | { type: 'delta' | 'verified'; data: { content: string } }
  | { type: 'verifying'; data: Record<string, never> }
  | { type: 'sources'; data: { items: KnowledgeSource[] } | KnowledgeSource[] }
  | { type: 'done'; data: { message_id: number; conversation_id: number; replayed?: boolean } }
  | {
      type: 'error';
      data: { code: string; message: string; incomplete?: boolean; conversation_id?: number };
    };

export type KnowledgeConversation = {
  id: number;
  title: string;
  source_scope: string;
  version?: number;
  updated_at?: string;
};

export type KnowledgeConversationDetail = KnowledgeConversation & {
  messages: Array<{
    id: number;
    role: 'user' | 'assistant';
    content: string;
    status?: string;
    request_id?: string;
  }>;
};

export class KnowledgeStreamError extends Error {
  constructor(
    message: string,
    readonly code: string,
    readonly details?: {
      clarification_id: string;
      conversation_id: number;
      kind: 'ambiguous' | 'scope_conflict';
      prompt: string;
      expires_at: string;
    },
  ) {
    super(message);
  }
}

export type KnowledgeList = {
  items: KnowledgeDocument[] | null;
  quota: {
    limit_bytes: number;
    used_bytes: number;
    reserved_bytes: number;
    remaining_bytes: number;
  };
};

export async function listKnowledge() {
  return (await http.get<KnowledgeList>('/api/v1/knowledge/documents', {})).data;
}
export async function uploadKnowledge(file: File) {
  const body = new FormData();
  body.append('file', file);
  return (await http.post('/api/v1/knowledge/uploads', body, {})).data;
}
export async function deleteKnowledge(id: string) {
  await http.delete(`/api/v1/knowledge/documents/${id}`, {});
}
export async function setNoteKnowledge(noteID: number, enabled: boolean) {
  return (await http.patch(`/api/v1/notes/${noteID}/knowledge`, { enabled }, {})).data;
}

export async function streamKnowledge(
  input: {
    question: string;
    request_id: string;
    conversation_id?: number;
    collection_ids?: string[];
    resume_clarification_id?: string;
    clarification?: string;
  },
  onEvent: (event: KnowledgeStreamEvent) => void,
  signal?: AbortSignal,
) {
  const response = await fetch('/api/v1/knowledge/chat/stream', {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
    signal,
  });
  if (!response.ok) {
    const error = await errorFromResponse(response);
    throw new KnowledgeStreamError(error.message, error.code, error.details as never);
  }
  await consumeSSE(response, ({ event, data }) => {
    if (event !== 'message')
      onEvent({ type: event, data: JSON.parse(data) } as KnowledgeStreamEvent);
  });
}

export async function listKnowledgeConversations() {
  return (
    await http.get<{ items: KnowledgeConversation[] }>('/api/v1/conversations', {
      params: { source_scope: 'knowledge' },
    })
  ).data;
}

export async function getKnowledgeConversation(id: number) {
  return (await http.get<KnowledgeConversationDetail>(`/api/v1/conversations/${id}`)).data;
}

export async function sendKnowledgeFeedback(requestID: string, category: string, comment = '') {
  return (
    await http.post(`/api/v1/knowledge/requests/${encodeURIComponent(requestID)}/feedback`, {
      category,
      comment,
    })
  ).data;
}
