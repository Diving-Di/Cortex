import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  listKnowledge,
  streamKnowledge,
  saveKnowledgeNote,
  retryKnowledge,
} from '../../api/knowledge';
import KnowledgePage from './KnowledgePage';

vi.mock('../../api/knowledge', () => ({
  listKnowledge: vi.fn(),
  saveKnowledgeNote: vi.fn().mockResolvedValue({ id: 91 }),
  getKnowledgeSources: vi.fn().mockResolvedValue([]),
  retryKnowledge: vi.fn().mockResolvedValue(undefined),
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

describe('KnowledgePage answer review', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockedListKnowledge.mockResolvedValue(response('success'));
    localStorage.setItem('cortex:guide:test-session:knowledge', 'seen');
  });
  afterEach(cleanup);
  it('shows the source excerpt and saves by server-owned message ID', async () => {
    vi.mocked(streamKnowledge).mockImplementation(async (_input, event) => {
      event({
        type: 'sources',
        data: {
          items: [
            {
              citation: 'K1',
              source_type: 'upload',
              document_id: 'doc',
              title: '参考资料',
              heading: ['第 3 页'],
              rank: 1,
              snippet: '真实原文片段',
              index_version: 2,
            },
          ],
        },
      });
      event({ type: 'verified', data: { content: '有依据的回答 [K1]' } });
      event({ type: 'done', data: { message_id: 42, conversation_id: 8 } });
    });
    renderPage();
    fireEvent.change(screen.getByPlaceholderText('根据我的知识库提问'), {
      target: { value: '总结资料' },
    });
    fireEvent.click(screen.getByText('发送'));
    fireEvent.click(await screen.findByText('来源（1）'));
    fireEvent.click(screen.getByText('参考资料'));
    expect(await screen.findByText('真实原文片段')).toBeInTheDocument();
    fireEvent.click(
      within(screen.getByRole('dialog', { name: '参考资料' })).getByRole('button', {
        name: 'Close',
      }),
    );
    fireEvent.click(screen.getByText('保存回答为笔记'));
    fireEvent.click(await screen.findByRole('button', { name: '确认保存回答' }));
    await waitFor(() => expect(saveKnowledgeNote).toHaveBeenCalledWith(42));
    expect(await screen.findByText('查看笔记')).toHaveAttribute('href', '/notes/91');
  });
  it('does not offer saving when the stream failed', async () => {
    vi.mocked(streamKnowledge).mockImplementation(async (_input, event) => {
      event({ type: 'delta', data: { content: '未完成' } });
      throw new Error('失败');
    });
    renderPage();
    fireEvent.change(screen.getByPlaceholderText('根据我的知识库提问'), {
      target: { value: '总结' },
    });
    fireEvent.click(screen.getByText('发送'));
    expect(await screen.findByText('回答不完整')).toBeInTheDocument();
    expect(screen.queryByText('保存回答为笔记')).not.toBeInTheDocument();
  });
  it('retries a failed rebuild while the old version remains readable', async () => {
    mockedListKnowledge.mockResolvedValue(response('failed', 'EMBEDDING_FAILED'));
    renderPage();
    fireEvent.click(await screen.findByText('重试索引'));
    await waitFor(() => expect(retryKnowledge).toHaveBeenCalledWith('document-id'));
    expect(screen.getByText('可用')).toBeInTheDocument();
  });
});
