import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import { expect, test, vi } from 'vitest';
import ResearchPage from './ResearchPage';

vi.mock('../../api/research', () => ({
  listResearchJobs: vi.fn().mockResolvedValue({ items: [], total: 0 }),
  listResearchSources: vi.fn().mockResolvedValue({ items: [], total: 0 }),
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
}));

vi.mock('../../api/knowledge', () => ({
  listKnowledgeCollections: vi.fn().mockResolvedValue([]),
}));

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
