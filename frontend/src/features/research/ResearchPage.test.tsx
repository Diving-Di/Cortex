import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, expect, test, vi } from 'vitest';
import ResearchPage from './ResearchPage';

const researchMocks = vi.hoisted(() => ({
  listResearchJobs: vi.fn(),
  listResearchSources: vi.fn(),
  getXHSAuthorization: vi.fn(),
  startXHSAuthorization: vi.fn(),
  createResearchJob: vi.fn(),
}));

vi.mock('../../api/research', () => ({
  listResearchJobs: researchMocks.listResearchJobs,
  listResearchSources: researchMocks.listResearchSources,
  getResearchSource: vi.fn(),
  createResearchJob: researchMocks.createResearchJob,
  cancelResearchJob: vi.fn(),
  retryResearchJob: vi.fn(),
  recollectResearchSource: vi.fn(),
  deleteResearchSource: vi.fn(),
  saveResearchSource: vi.fn(),
  ignoreResearchSource: vi.fn(),
  batchSaveResearchSources: vi.fn(),
  batchIgnoreResearchSources: vi.fn(),
  updateResearchDraft: vi.fn(),
  loadResearchAsset: vi.fn(),
  getXHSAuthorization: researchMocks.getXHSAuthorization,
  startXHSAuthorization: researchMocks.startXHSAuthorization,
  getXHSAuthAttempt: vi.fn(),
  loadXHSAuthQR: vi.fn(),
  cancelXHSAuthorization: vi.fn(),
  verifyXHSAuthorization: vi.fn(),
  revokeXHSAuthorization: vi.fn(),
}));

beforeEach(() => {
  cleanup();
  vi.clearAllMocks();
  researchMocks.listResearchJobs.mockResolvedValue({ items: [], total: 0 });
  researchMocks.listResearchSources.mockResolvedValue({ items: [], total: 0 });
  researchMocks.getXHSAuthorization.mockResolvedValue({
    id: 1,
    status: 'authorized',
    version: 1,
  });
});

test('renders research navigation content and empty jobs', async () => {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <ResearchPage />
    </QueryClientProvider>,
  );
  expect(await screen.findByRole('heading', { name: '小红书研究' })).toBeInTheDocument();
  expect(screen.getByRole('button', { name: /新建研究/ })).toBeInTheDocument();
  expect(await screen.findByText('尚未创建研究任务')).toBeInTheDocument();
});

test('keeps rendering for an authorized tenant when legacy empty lists are null', async () => {
  researchMocks.listResearchJobs.mockResolvedValue({ items: null, total: 0 });
  researchMocks.listResearchSources.mockResolvedValue({ items: null, total: 0 });

  render(
    <QueryClientProvider client={new QueryClient()}>
      <ResearchPage />
    </QueryClientProvider>,
  );

  expect(await screen.findByRole('heading', { name: '小红书研究' })).toBeInTheDocument();
  expect(await screen.findByText('尚未创建研究任务')).toBeInTheDocument();
  expect(screen.getByRole('button', { name: /新建研究/ })).toBeInTheDocument();
});

test('keeps rendering when the server returns an unfamiliar status', async () => {
  researchMocks.listResearchJobs.mockResolvedValue({
    items: [
      {
        id: 1,
        mode: 'keyword',
        query_payload: { keywords: ['测试'] },
        target_count: 1,
        status: 'unexpected_status',
        found_count: 0,
        collected_count: 0,
        organized_count: 0,
        failed_count: 0,
        saved_count: 0,
        attempt_count: 0,
        max_attempts: 3,
        created_at: '2026-07-28T00:00:00Z',
        updated_at: '2026-07-28T00:00:00Z',
      },
    ],
    total: 1,
  });

  render(
    <QueryClientProvider client={new QueryClient()}>
      <ResearchPage />
    </QueryClientProvider>,
  );

  expect(await screen.findByText('未知状态')).toBeInTheDocument();
});

test('shows the authorization gate before loading the research workspace', async () => {
  researchMocks.getXHSAuthorization.mockResolvedValue(null);

  render(
    <QueryClientProvider client={new QueryClient()}>
      <ResearchPage />
    </QueryClientProvider>,
  );

  expect(await screen.findByRole('heading', { name: '授权小红书账号' })).toBeInTheDocument();
  expect(screen.getByRole('button', { name: '打开扫码授权窗口' })).toBeInTheDocument();
  expect(screen.queryByRole('button', { name: /新建研究/ })).not.toBeInTheDocument();
  expect(researchMocks.listResearchJobs).not.toHaveBeenCalled();
  expect(researchMocks.listResearchSources).not.toHaveBeenCalled();
});

test('requires a new scan for a legacy authorized session', async () => {
  researchMocks.getXHSAuthorization.mockResolvedValue({
    id: 1,
    status: 'authorized',
    requires_reauthorization: true,
    version: 2,
  });

  render(
    <QueryClientProvider client={new QueryClient()}>
      <ResearchPage />
    </QueryClientProvider>,
  );

  expect(
    await screen.findByRole('heading', { name: '需要重新授权小红书账号' }),
  ).toBeInTheDocument();
  expect(screen.getByText('现有授权缺少采集所需的新版浏览器状态')).toBeInTheDocument();
  expect(researchMocks.listResearchJobs).not.toHaveBeenCalled();
});

test('creates a keyword research job from the modal', async () => {
  researchMocks.createResearchJob.mockResolvedValue({ id: 9, status: 'queued' });
  render(
    <QueryClientProvider client={new QueryClient()}>
      <ResearchPage />
    </QueryClientProvider>,
  );
  fireEvent.click(await screen.findByRole('button', { name: /新建研究/ }));
  fireEvent.change(screen.getByLabelText('研究关键词'), {
    target: { value: 'Agent 面试\nRAG 实践' },
  });
  fireEvent.click(screen.getByRole('button', { name: '开始研究' }));

  await waitFor(() =>
    expect(researchMocks.createResearchJob).toHaveBeenCalledWith(
      expect.objectContaining({
        mode: 'keyword',
        keywords: ['Agent 面试', 'RAG 实践'],
        target_count: 10,
        idempotency_key: expect.any(String),
      }),
    ),
  );
});

test('shows a recoverable page error when jobs fail to load', async () => {
  researchMocks.listResearchJobs.mockRejectedValue(new Error('offline'));
  render(
    <QueryClientProvider
      client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}
    >
      <ResearchPage />
    </QueryClientProvider>,
  );

  expect(await screen.findByText('研究页面数据加载失败')).toBeInTheDocument();
  expect(screen.getByRole('button', { name: /重\s*试/ })).toBeInTheDocument();
});
