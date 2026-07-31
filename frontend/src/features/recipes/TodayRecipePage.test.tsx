import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import TodayRecipePage from './TodayRecipePage';

vi.mock('../../api/recipes', () => ({
  getTodayRecipe: vi.fn(async () => ({
    local_date: '2026-07-30',
    timezone: 'Asia/Shanghai',
    corpus_revision: 'revision',
    recipe: {
      id: 7,
      title: '红烧鱼',
      category: 'aquatic',
      summary: '一道家常菜',
      ingredients: ['鱼', '姜'],
      dietary_warnings: [],
    },
    suggested_questions: ['需要哪些食材和用量？', '请完整说明制作步骤。', '有哪些技巧？'],
  })),
  getRecipePreferences: vi.fn(async () => ({
    dietary_restrictions: ['花生'],
    timezone: 'Asia/Shanghai',
    version: 1,
  })),
  updateRecipePreferences: vi.fn(),
  listRecipeConversations: vi.fn(async () => [
    {
      id: 12,
      title: '微波炉蛋糕',
      source_scope: 'recipe',
      message_count: 2,
      version: 1,
    },
  ]),
  getRecipeConversation: vi.fn(async () => ({
    id: 12,
    title: '微波炉蛋糕',
    source_scope: 'recipe',
    message_count: 2,
    version: 1,
    messages: [
      { id: 1, role: 'user', content: '怎么做？' },
      { id: 2, role: 'assistant', content: '混合后加热。' },
    ],
  })),
}));

beforeEach(() => vi.clearAllMocks());
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

test('renders the daily recipe, three questions and free input', async () => {
  render(<TodayRecipePage token="token" />);
  expect(await screen.findByText('今日菜谱：红烧鱼')).toBeInTheDocument();
  expect(screen.getByText('鱼、姜')).toBeInTheDocument();
  expect(screen.getAllByRole('button', { name: /食材|步骤|技巧/ })).toHaveLength(3);
  expect(screen.getByLabelText('烹饪问题')).toBeInTheDocument();
  expect(screen.getByRole('button', { name: /新聊天/ })).toBeInTheDocument();
  expect(screen.getByText('微波炉蛋糕')).toBeInTheDocument();
  fireEvent.click(screen.getByText('微波炉蛋糕'));
  expect(await screen.findByText('混合后加热。')).toBeInTheDocument();
  fireEvent.click(screen.getByRole('button', { name: /新聊天/ }));
  expect(screen.queryByText('混合后加热。')).not.toBeInTheDocument();
});

test('renders streamed recipe answers as Markdown', async () => {
  const body = [
    'event: delta\ndata: {"content":"## 制作步骤\\n\\n"}\n\n',
    'event: delta\ndata: {"content":"1. **混合**原料\\n2. 放入微波炉"}\n\n',
  ].join('');
  vi.stubGlobal(
    'fetch',
    vi.fn(async () => ({
      ok: true,
      body: new Response(body).body,
    })),
  );

  render(<TodayRecipePage token="token" />);
  await screen.findByText('今日菜谱：红烧鱼');
  fireEvent.change(screen.getByLabelText('烹饪问题'), {
    target: { value: '怎么制作？' },
  });
  fireEvent.click(screen.getByRole('button', { name: '发送' }));

  expect(await screen.findByRole('heading', { name: '制作步骤', level: 2 })).toBeInTheDocument();
  expect(screen.getByText('混合').tagName).toBe('STRONG');
  await waitFor(() => expect(screen.getByLabelText('建议问题')).toBeInTheDocument());
  expect(
    screen.getByText('混合').closest('.recipe-message')?.querySelectorAll('ol > li'),
  ).toHaveLength(2);
});
