import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import KnowledgePage from './KnowledgePage';

const knowledgeMocks = vi.hoisted(() => ({
  getKnowledgeDocument: vi.fn(),
}));

vi.mock('../../api/knowledge', () => ({
  listKnowledgeCollections: vi.fn().mockResolvedValue([{ id: 1, name: '工作', version: 1 }]),
  listKnowledgeDocuments: vi.fn().mockResolvedValue({ items: [], total: 0 }),
  createKnowledgeCollection: vi.fn(),
  deleteKnowledgeCollection: vi.fn(),
  deleteKnowledgeDocument: vi.fn(),
  downloadKnowledgeDocument: vi.fn(),
  getKnowledgePreview: vi.fn().mockResolvedValue({ data: '引用预览' }),
  getKnowledgeDocument: knowledgeMocks.getKnowledgeDocument,
  reindexKnowledgeDocument: vi.fn(),
  uploadKnowledgeDocument: vi.fn(),
}));

afterEach(() => {
  cleanup();
  window.history.replaceState({}, '', '/');
  vi.clearAllMocks();
});

test('renders accessible knowledge filters and empty state', async () => {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <KnowledgePage token="test-token" />
    </QueryClientProvider>,
  );
  expect(screen.getByRole('heading', { name: '个人知识库' })).toBeInTheDocument();
  expect(screen.getByText(/支持 \.txt、\.md、\.pdf、\.docx/)).toBeInTheDocument();
  expect(screen.getByLabelText('搜索文件名')).toBeInTheDocument();
  expect(screen.getAllByLabelText('筛选处理状态').length).toBeGreaterThan(0);
  expect(await screen.findByText('尚未上传知识文件')).toBeInTheDocument();
});

test('opens a cited document from the query string', async () => {
  knowledgeMocks.getKnowledgeDocument.mockResolvedValue({
    id: 42,
    original_name: '项目复盘.txt',
    extension: '.txt',
    status: 'ready',
    character_count: 120,
    page_count: 1,
    size: 120,
    sha256: 'a'.repeat(64),
    index_version: 1,
    created_at: '2026-07-29T00:00:00Z',
    updated_at: '2026-07-29T00:00:00Z',
  });
  window.history.replaceState({}, '', '/knowledge?document_id=42');

  render(
    <QueryClientProvider client={new QueryClient()}>
      <KnowledgePage token="test-token" />
    </QueryClientProvider>,
  );

  expect(await screen.findByText('项目复盘.txt')).toBeInTheDocument();
  expect(knowledgeMocks.getKnowledgeDocument).toHaveBeenCalledWith('test-token', 42);
});
