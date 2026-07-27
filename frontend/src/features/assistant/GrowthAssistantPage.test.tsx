import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
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

test('renders source controls, stop-capable chat, and non-ready exclusion', async () => {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <GrowthAssistantPage token="test-token" />
    </QueryClientProvider>,
  );
  expect(screen.getByRole('heading', { name: '成长助手' })).toBeInTheDocument();
  expect(screen.getAllByLabelText('来源范围').length).toBeGreaterThan(0);
  expect(screen.getByLabelText('输入问题')).toBeInTheDocument();
  expect(await screen.findByText('1 个未 ready 文件已排除')).toBeInTheDocument();
});
