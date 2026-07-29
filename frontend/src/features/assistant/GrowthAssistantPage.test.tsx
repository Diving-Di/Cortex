import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import GrowthAssistantPage from './GrowthAssistantPage';

const knowledgeMocks = vi.hoisted(() => ({
  createConversation: vi.fn(),
  listConversations: vi.fn(),
}));

vi.mock('../../api/knowledge', () => ({
  listConversations: knowledgeMocks.listConversations,
  createConversation: knowledgeMocks.createConversation,
  deleteConversation: vi.fn(),
  getConversation: vi.fn(),
  renameConversation: vi.fn(),
}));

beforeEach(() => {
  knowledgeMocks.listConversations.mockResolvedValue([]);
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

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
  expect(screen.getByRole('heading', { name: '成长助手' })).toBeInTheDocument();
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
  expect(screen.getByLabelText('折叠会话列表').closest('.growth-chat-card')).not.toBeNull();
});

test('renders SSE citations as navigable source links', async () => {
  const stream = [
    'event: delta\ndata: {"content":"有依据的回答"}\n\n',
    'event: sources\ndata: {"items":[{"source_type":"knowledge_document","source_id":42,"title":"项目复盘","snippet":"引用内容"}]}\n\n',
    'event: done\ndata: {"conversation_id":7}\n\n',
  ].join('');
  vi.spyOn(globalThis, 'fetch').mockResolvedValue(
    new Response(new TextEncoder().encode(stream), {
      status: 200,
      headers: { 'Content-Type': 'text/event-stream' },
    }),
  );
  render(
    <QueryClientProvider client={new QueryClient()}>
      <GrowthAssistantPage token="test-token" />
    </QueryClientProvider>,
  );

  fireEvent.change(screen.getByLabelText('输入问题'), { target: { value: '项目结果是什么？' } });
  fireEvent.click(screen.getByRole('button', { name: '发送' }));

  const citation = await screen.findByRole('link', { name: '查看引用：项目复盘' });
  expect(citation).toHaveAttribute('href', '/knowledge?document_id=42');
  expect(await screen.findByText('有依据的回答')).toBeInTheDocument();
});
