import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen } from '@testing-library/react';
import { expect, test, vi } from 'vitest';
import GrowthAssistantPage from './GrowthAssistantPage';

vi.mock('../../api/knowledge', () => ({
  listConversations: vi.fn().mockResolvedValue([]),
  listKnowledgeCollections: vi.fn().mockResolvedValue([]),
  listKnowledgeDocuments: vi.fn().mockResolvedValue({
    items: [{ id: 1, original_name: '处理中.pdf', status: 'indexing' }],
    total: 1,
  }),
  createConversation: vi.fn(),
  deleteConversation: vi.fn(),
  getConversation: vi.fn(),
}));

test('offers only knowledge base and notebook sources', async () => {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <GrowthAssistantPage token="test-token" />
    </QueryClientProvider>,
  );
  expect(screen.getByRole('heading', { name: '成长助手' })).toBeInTheDocument();
  const sourceSelect = screen.getByRole('combobox', { name: '来源范围' });
  fireEvent.mouseDown(sourceSelect);
  expect(await screen.findByText('笔记本')).toBeInTheDocument();
  expect(screen.queryByText('成长记录')).not.toBeInTheDocument();
  expect(screen.queryByText('全部来源')).not.toBeInTheDocument();
  expect(screen.getByLabelText('输入问题')).toBeInTheDocument();
  expect(await screen.findByText('1 个未 ready 文件已排除')).toBeInTheDocument();
});
