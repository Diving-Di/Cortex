import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import { writeDraft } from '../../app/drafts';
import { retryKnowledge } from '../../api/knowledge';
import PendingWork from './PendingWork';
vi.mock('../../api/knowledge', () => ({
  listKnowledge: vi.fn().mockResolvedValue({
    items: [{ id: 'doc', Title: '失败资料', Status: 'failed', failure_summary: '解析失败' }],
  }),
  retryKnowledge: vi.fn().mockResolvedValue(undefined),
}));
vi.mock('../../api/scheduledReports', () => ({
  listScheduledReports: vi.fn().mockResolvedValue([]),
  listScheduledReportRuns: vi.fn(),
  retryScheduledReport: vi.fn(),
}));
vi.mock('../../api/notes', () => ({ listNotes: vi.fn().mockResolvedValue({ items: [] }) }));
afterEach(() => {
  cleanup();
  sessionStorage.clear();
});
test('links the owned drafts and retries failed indexing from the workbench', async () => {
  writeDraft('test-session', {
    key: 'note:7',
    label: '待确认记录',
    path: '/notes/7',
    value: { content: 'draft' },
  });
  writeDraft('other', {
    key: 'note:8',
    label: '其他账号记录',
    path: '/notes/8',
    value: { content: 'private' },
  });
  render(
    <QueryClientProvider client={new QueryClient()}>
      <PendingWork />
    </QueryClientProvider>,
  );
  expect(screen.getByText('继续：待确认记录')).toHaveAttribute('href', '/notes/7');
  expect(screen.queryByText('其他账号记录')).not.toBeInTheDocument();
  expect(await screen.findByText('失败资料 · 解析失败')).toBeInTheDocument();
  fireEvent.click(screen.getByRole('button', { name: /重\s*试/ }));
  await waitFor(() => expect(retryKnowledge).toHaveBeenCalledWith('doc'));
});
