import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import { beforeEach, expect, test, vi } from 'vitest';
import ResearchPage from './ResearchPage';

const researchMocks = vi.hoisted(() => ({
  listResearchJobs: vi.fn(),
  listResearchSources: vi.fn(),
}));

vi.mock('../../api/research', () => ({
  listResearchJobs: researchMocks.listResearchJobs,
  listResearchSources: researchMocks.listResearchSources,
  getResearchSource: vi.fn(),
  createResearchJob: vi.fn(),
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
  getXHSAuthorization: vi.fn().mockResolvedValue(null),
  startXHSAuthorization: vi.fn(),
  getXHSAuthAttempt: vi.fn(),
  loadXHSAuthQR: vi.fn(),
  cancelXHSAuthorization: vi.fn(),
  verifyXHSAuthorization: vi.fn(),
  revokeXHSAuthorization: vi.fn(),
}));

vi.mock('../../api/knowledge', () => ({
  listKnowledgeCollections: vi.fn().mockResolvedValue([]),
}));

beforeEach(() => {
  researchMocks.listResearchJobs.mockResolvedValue({ items: [], total: 0 });
  researchMocks.listResearchSources.mockResolvedValue({ items: [], total: 0 });
});

test('renders research navigation content and empty jobs', async () => {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <ResearchPage token="test-token" />
    </QueryClientProvider>,
  );
  expect(screen.getByRole('heading', { name: '小红书研究' })).toBeInTheDocument();
  expect(screen.getByRole('button', { name: /新建研究/ })).toBeInTheDocument();
  expect(await screen.findByText('尚未创建研究任务')).toBeInTheDocument();
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
      <ResearchPage token="test-token" />
    </QueryClientProvider>,
  );

  expect(await screen.findByText('未知状态')).toBeInTheDocument();
});
