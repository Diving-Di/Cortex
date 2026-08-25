import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { listKnowledge } from '../../api/knowledge';
import KnowledgePage from './KnowledgePage';

vi.mock('../../api/knowledge', () => ({
  listKnowledge: vi.fn(),
  uploadKnowledge: vi.fn(),
  deleteKnowledge: vi.fn(),
  listKnowledgeConversations: vi.fn().mockResolvedValue({ items: [], total: 0 }),
  getKnowledgeConversation: vi.fn(),
  sendKnowledgeFeedback: vi.fn(),
  streamKnowledge: vi.fn(),
  KnowledgeStreamError: class extends Error {},
}));

const mockedListKnowledge = vi.mocked(listKnowledge);

function renderPage() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <KnowledgePage />
    </QueryClientProvider>,
  );
}

function response(indexJobStatus: 'running' | 'failed' | 'success', failure?: string) {
  return {
    items: [
      {
        id: 'document-id',
        SourceType: 'upload' as const,
        Title: '版本化文档',
        Status: 'ready' as const,
        size_bytes: 1024,
        active_index_version: 2,
        index_job_status: indexJobStatus,
        last_index_failure_code: failure,
        CreatedAt: '2026-08-11T00:00:00Z',
        UpdatedAt: '2026-08-11T00:00:00Z',
      },
    ],
    quota: {
      limit_bytes: 3221225472,
      used_bytes: 1024,
      reserved_bytes: 0,
      remaining_bytes: 3221224448,
    },
  };
}

describe('KnowledgePage index serving state', () => {
  beforeEach(() => mockedListKnowledge.mockReset());
  afterEach(cleanup);

  it('keeps an active document available while its next index is running', async () => {
    mockedListKnowledge.mockResolvedValue(response('running'));
    renderPage();
    expect(await screen.findByText('可用，正在更新索引')).toBeInTheDocument();
    expect(screen.getByText('可用')).toBeInTheDocument();
  });

  it('shows a rebuild failure without marking the active document unavailable', async () => {
    mockedListKnowledge.mockResolvedValue(response('failed', 'KNOWLEDGE_EMBEDDING_UNAVAILABLE'));
    renderPage();
    expect(
      await screen.findByText('旧版本可用，最近更新失败：KNOWLEDGE_EMBEDDING_UNAVAILABLE'),
    ).toBeInTheDocument();
    expect(screen.getByText('可用')).toBeInTheDocument();
  });
});

describe('KnowledgePage upload formats', () => {
  beforeEach(() => mockedListKnowledge.mockReset());
  afterEach(cleanup);

  it('advertises binary document and image ingestion', async () => {
    mockedListKnowledge.mockResolvedValue(response('success'));
    renderPage();
    expect(await screen.findByText('支持 Markdown、PDF、Word 和图片')).toBeInTheDocument();
    const input = document.querySelector('input[type="file"]');
    expect(input).toHaveAttribute('accept', '.md,.zip,.pdf,.doc,.docx,.png,.jpg,.jpeg,.webp');
  });
});
