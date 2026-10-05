import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import {
  createReportJob,
  listReportJobs,
  getReportJob,
  confirmReportJob,
  ReportJob,
} from '../../api/reportJobs';
import BackgroundReports from './BackgroundReports';

vi.mock('../../api/reportJobs', () => ({
  createReportJob: vi.fn(),
  listReportJobs: vi.fn(),
  getReportJob: vi.fn(),
  confirmReportJob: vi.fn(),
  cancelReportJob: vi.fn(),
}));
const job: ReportJob = {
  id: 'job-1',
  request_id: 'request-1',
  type: 'monthly',
  anchor_date: '2026-10-01',
  status: 'success',
  stage: 'success',
  content: '服务端草稿 [#7]',
  failure_code: null,
  confirmed_note_id: null,
  created_at: '2026-10-05T00:00:00Z',
  updated_at: '2026-10-05T00:00:00Z',
  sources: [{ id: 7, title: '来源笔记', note_date: '2026-10-05', snippet: '来源' }],
};
function page() {
  return render(
    <QueryClientProvider
      client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}
    >
      <BackgroundReports type="monthly" anchorDate="2026-10-01" />
    </QueryClientProvider>,
  );
}
beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(listReportJobs).mockResolvedValue([]);
  vi.mocked(getReportJob).mockResolvedValue(job);
  vi.mocked(createReportJob).mockResolvedValue(job);
  vi.mocked(confirmReportJob).mockResolvedValue({ id: 9 });
});
afterEach(cleanup);

test('loads persisted drafts after a new page mount and confirms the job ID', async () => {
  vi.mocked(listReportJobs).mockResolvedValue([job]);
  const first = page();
  await screen.findByText('核对草稿');
  first.unmount();
  page();
  fireEvent.click(await screen.findByText('核对草稿'));
  expect(await screen.findByDisplayValue('服务端草稿 [#7]')).toBeInTheDocument();
  fireEvent.click(screen.getByText('确认保存报告'));
  await waitFor(() =>
    expect(confirmReportJob).toHaveBeenCalledWith(
      'job-1',
      expect.objectContaining({ content: '服务端草稿 [#7]', overwrite: false }),
    ),
  );
});

test('reuses the idempotency key after a lost submission response', async () => {
  vi.mocked(createReportJob)
    .mockRejectedValueOnce(new Error('连接中断'))
    .mockResolvedValueOnce(job);
  page();
  fireEvent.click(screen.getByText('后台生成草稿'));
  await screen.findByText('连接中断');
  await waitFor(() =>
    expect(screen.getByText('后台生成草稿').closest('button')).not.toHaveClass('ant-btn-loading'),
  );
  fireEvent.click(screen.getByText('后台生成草稿'));
  await waitFor(() => expect(createReportJob).toHaveBeenCalledTimes(2));
  const calls = vi.mocked(createReportJob).mock.calls;
  expect(calls[0][0].request_id).toBe(calls[1][0].request_id);
});

test('shows failed attempts without offering confirmation', async () => {
  vi.mocked(listReportJobs).mockResolvedValue([
    { ...job, status: 'failed', failure_code: 'REPORT_WORKER_INTERRUPTED' },
  ]);
  page();
  expect(await screen.findByText('执行中断，请手动重试')).toBeInTheDocument();
  expect(screen.queryByText('核对草稿')).not.toBeInTheDocument();
  fireEvent.click(screen.getByText('重新生成'));
  await waitFor(() =>
    expect(createReportJob).toHaveBeenCalledWith(
      expect.objectContaining({ type: 'monthly', anchor_date: '2026-10-01' }),
    ),
  );
});
