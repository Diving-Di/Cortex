import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import GrowthAssistantPage from './GrowthAssistantPage';

const knowledgeMocks = vi.hoisted(() => ({
  createConversation: vi.fn(),
}));

vi.mock('../../api/knowledge', () => ({
  listConversations: vi.fn().mockResolvedValue([]),
  createConversation: knowledgeMocks.createConversation,
  deleteConversation: vi.fn(),
  getConversation: vi.fn(),
  renameConversation: vi.fn(),
}));

afterEach(cleanup);

test('offers only knowledge base and notebook sources', async () => {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <GrowthAssistantPage token="test-token" />
    </QueryClientProvider>,
  );
  expect(screen.queryByText('回答严格依据你选择的个人来源。')).not.toBeInTheDocument();
  const sourceSelect = screen.getByRole('combobox', { name: '来源范围' });
  fireEvent.mouseDown(sourceSelect);
  expect(await screen.findByText('笔记本')).toBeInTheDocument();
  expect(screen.queryByText('成长记录')).not.toBeInTheDocument();
  expect(screen.queryByText('全部来源')).not.toBeInTheDocument();
  expect(screen.queryByLabelText('搜索会话')).not.toBeInTheDocument();
  expect(screen.queryByLabelText('选择知识集合')).not.toBeInTheDocument();
  expect(screen.queryByLabelText('选择知识文件')).not.toBeInTheDocument();
  expect(screen.getByLabelText('输入问题')).toBeInTheDocument();
  expect(screen.getByText('有什么想了解的？')).toBeInTheDocument();
});

test('starts a clean local conversation without creating a persisted record', () => {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <GrowthAssistantPage token="test-token" />
    </QueryClientProvider>,
  );

  fireEvent.click(screen.getByRole('button', { name: /新建会话/ }));

  expect(knowledgeMocks.createConversation).not.toHaveBeenCalled();
  expect(screen.getByLabelText('折叠会话列表')).toBeInTheDocument();
});
