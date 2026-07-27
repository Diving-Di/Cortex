import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import { expect, test, vi } from 'vitest';
import KnowledgePage from './KnowledgePage';

vi.mock('../../api/knowledge', () => ({
  listKnowledgeCollections: vi.fn().mockResolvedValue([{ id: 1, name: '工作', version: 1 }]),
  listKnowledgeDocuments: vi.fn().mockResolvedValue({ items: [], total: 0 }),
  createKnowledgeCollection: vi.fn(),
  deleteKnowledgeCollection: vi.fn(),
  deleteKnowledgeDocument: vi.fn(),
  downloadKnowledgeDocument: vi.fn(),
  getKnowledgePreview: vi.fn(),
  reindexKnowledgeDocument: vi.fn(),
  uploadKnowledgeDocument: vi.fn(),
}));

test('renders accessible knowledge filters and empty state', async () => {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <KnowledgePage token="test-token" />
    </QueryClientProvider>,
  );
  expect(screen.getByRole('heading', { name: '个人知识库' })).toBeInTheDocument();
  expect(screen.getByLabelText('搜索文件名')).toBeInTheDocument();
  expect(screen.getAllByLabelText('筛选处理状态').length).toBeGreaterThan(0);
  expect(await screen.findByText('尚未上传知识文件')).toBeInTheDocument();
});
