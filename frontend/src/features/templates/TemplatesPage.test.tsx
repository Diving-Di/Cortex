import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, expect, test, vi } from 'vitest';
import TemplatesPage from './TemplatesPage';
import {
  getPublicTemplate,
  listMyTemplates,
  listPublicTemplates,
  recordTemplateView,
  useTemplate,
} from '../../api/templates';

vi.mock('../../api/templates', () => ({
  getPublicTemplate: vi.fn(),
  listPublicTemplates: vi.fn().mockResolvedValue({
    items: [
      {
        public_id: 'p1',
        author_nickname: '作者',
        version: 1,
        title: '每日复盘',
        description: '说明',
        content_markdown: '# 内容',
        category: '复盘',
        published_at: '2026-08-01T00:00:00Z',
        like_count: 1,
        favorite_count: 2,
        usage_count: 3,
        liked: false,
        favorited: false,
      },
    ],
    next_cursor: '',
  }),
  listMyTemplates: vi.fn().mockResolvedValue([]),
  createTemplate: vi.fn(),
  deleteTemplate: vi.fn(),
  publishTemplate: vi.fn(),
  recordTemplateView: vi.fn(),
  savePublicProfile: vi.fn(),
  useTemplate: vi.fn(),
  usePrivateTemplate: vi.fn(),
  withdrawTemplate: vi.fn(),
  setTemplateReaction: vi.fn(),
  updateTemplate: vi.fn(),
}));
afterEach(cleanup);
test('renders public templates with interactions', async () => {
  vi.mocked(getPublicTemplate).mockResolvedValueOnce({
    public_id: 'p1',
    author_nickname: '作者',
    version: 1,
    title: '每日复盘',
    description: '说明',
    content_markdown: '# 详情内容',
    category: '复盘',
    published_at: '2026-08-01T00:00:00Z',
    like_count: 1,
    favorite_count: 2,
    usage_count: 3,
    liked: false,
    favorited: false,
  });
  render(
    <QueryClientProvider client={new QueryClient()}>
      <MemoryRouter>
        <TemplatesPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  expect(await screen.findByText('每日复盘')).toBeInTheDocument();
  expect(screen.getByText(/使用模板/)).toBeInTheDocument();
  expect(screen.getByText(/点赞/)).toBeInTheDocument();
  expect(screen.getByText(/收藏/)).toBeInTheDocument();
  expect(screen.queryByText('内容')).not.toBeInTheDocument();
  fireEvent.click(screen.getByRole('button', { name: '查看详情' }));
  expect(await screen.findByText('详情内容')).toBeInTheDocument();
  expect(getPublicTemplate).toHaveBeenCalledWith('p1');
  await waitFor(() => expect(recordTemplateView).toHaveBeenCalledWith('p1'));
});

test('reuses the creation idempotency key after failure and disables creation while pending', async () => {
  let rejectFirst!: (reason?: unknown) => void;
  vi.mocked(useTemplate)
    .mockReset()
    .mockImplementationOnce(
      () =>
        new Promise((_, reject) => {
          rejectFirst = reject;
        }),
    )
    .mockResolvedValueOnce({ note_id: 42 });
  render(
    <QueryClientProvider
      client={new QueryClient({ defaultOptions: { mutations: { retry: false } } })}
    >
      <MemoryRouter>
        <TemplatesPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );

  const useButton = await screen.findByRole('button', { name: '使用模板' });
  fireEvent.click(useButton);
  await waitFor(() => expect(useButton).toBeDisabled());
  const firstKey = vi.mocked(useTemplate).mock.calls[0][1];

  rejectFirst(new Error('network error'));
  await waitFor(() => expect(useButton).toBeEnabled());
  fireEvent.click(useButton);

  await waitFor(() => expect(useTemplate).toHaveBeenCalledTimes(2));
  expect(useTemplate).toHaveBeenNthCalledWith(1, 'p1', firstKey);
  expect(useTemplate).toHaveBeenNthCalledWith(2, 'p1', firstKey);
});

test('loads the next signed-cursor page', async () => {
  vi.mocked(listPublicTemplates)
    .mockReset()
    .mockResolvedValueOnce({
      items: [
        {
          public_id: 'p1',
          author_nickname: '作者一',
          version: 1,
          title: '第一页模板',
          description: '',
          content_markdown: '# 一',
          category: '复盘',
          published_at: '2026-08-01T00:00:00Z',
          like_count: 0,
          favorite_count: 0,
          usage_count: 0,
          liked: false,
          favorited: false,
        },
      ],
      next_cursor: 'signed-cursor',
    })
    .mockResolvedValueOnce({
      items: [
        {
          public_id: 'p2',
          author_nickname: '作者二',
          version: 1,
          title: '第二页模板',
          description: '',
          content_markdown: '# 二',
          category: '计划',
          published_at: '2026-07-31T00:00:00Z',
          like_count: 0,
          favorite_count: 0,
          usage_count: 0,
          liked: false,
          favorited: false,
        },
      ],
      next_cursor: '',
    });
  render(
    <QueryClientProvider client={new QueryClient()}>
      <MemoryRouter>
        <TemplatesPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  expect(await screen.findByText('第一页模板')).toBeInTheDocument();
  fireEvent.click(screen.getByText('加载更多'));
  expect(await screen.findByText('第二页模板')).toBeInTheDocument();
  expect(listPublicTemplates).toHaveBeenLastCalledWith('recommended', 'signed-cursor', '', '');
});

test('searches public templates through the list API', async () => {
  vi.mocked(listPublicTemplates).mockResolvedValue({
    items: [
      {
        public_id: 'p1',
        author_nickname: '作者',
        version: 1,
        title: '每日复盘',
        description: '说明',
        content_markdown: '# 内容',
        category: '复盘',
        published_at: '2026-08-01T00:00:00Z',
        like_count: 1,
        favorite_count: 2,
        usage_count: 3,
        liked: false,
        favorited: false,
      },
    ],
    next_cursor: '',
  });
  render(
    <QueryClientProvider client={new QueryClient()}>
      <MemoryRouter>
        <TemplatesPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  expect(await screen.findByText('每日复盘')).toBeInTheDocument();
  fireEvent.change(screen.getByRole('searchbox', { name: '搜索模板' }), {
    target: { value: '周报' },
  });
  fireEvent.keyDown(screen.getByRole('searchbox', { name: '搜索模板' }), { key: 'Enter' });
  await waitFor(() =>
    expect(listPublicTemplates).toHaveBeenLastCalledWith('recommended', '', '周报', ''),
  );
});

test('renders an empty private-template list when the legacy API returns null', async () => {
  vi.mocked(listMyTemplates).mockResolvedValueOnce(null as never);
  render(
    <QueryClientProvider client={new QueryClient()}>
      <MemoryRouter>
        <TemplatesPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  fireEvent.click(screen.getByRole('tab', { name: '我的模板' }));
  expect(await screen.findByRole('button', { name: '新建模板' })).toBeInTheDocument();
});
