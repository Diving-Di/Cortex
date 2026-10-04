import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { previewReport, streamPost, confirmReport } from '../../api/m2';
import ReportsPage from './ReportsPage';
vi.mock('../../api/m2', () => ({
  previewReport: vi.fn(),
  streamPost: vi.fn(),
  confirmReport: vi.fn(),
}));
vi.mock('../../api/notes', () => ({
  listNotes: vi.fn().mockResolvedValue({ items: [], total: 0 }),
}));
vi.mock('../../api/scheduledReports', () => ({
  listScheduledReports: vi.fn().mockResolvedValue([]),
  createScheduledReport: vi.fn(),
  listScheduledReportRuns: vi.fn(),
  retryScheduledReport: vi.fn(),
  setScheduledReportEnabled: vi.fn(),
}));
vi.mock('../../components/UsageGuide', () => ({ default: () => null }));
const source = { id: 1, title: '原来源', note_date: '2026-10-05', snippet: '原文' };
function page() {
  return render(
    <MemoryRouter>
      <QueryClientProvider
        client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}
      >
        <ReportsPage />
      </QueryClientProvider>
    </MemoryRouter>,
  );
}
beforeEach(() => {
  vi.clearAllMocks();
  sessionStorage.clear();
  vi.mocked(previewReport).mockResolvedValue({
    sources: [source],
    start_date: '2026-10-05',
    end_date: '2026-10-11',
  });
});
afterEach(cleanup);
async function generate() {
  fireEvent.click(screen.getByText('选择来源'));
  await waitFor(() => expect(screen.getByText('生成草稿').closest('button')).not.toBeDisabled());
  fireEvent.click(screen.getByText('生成草稿'));
}
test('binds save to the actual generation sources and restores a complete draft', async () => {
  const actual = { ...source, id: 2, title: '实际生成来源' };
  vi.mocked(streamPost).mockImplementation(async (_p, _b, chunk, options) => {
    options?.onSources?.([actual]);
    chunk('完整报告');
  });
  const first = page();
  await generate();
  expect(await screen.findByDisplayValue('完整报告')).toBeInTheDocument();
  first.unmount();
  page();
  expect(await screen.findByDisplayValue('完整报告')).toBeInTheDocument();
  fireEvent.click(screen.getByText('确认保存'));
  await waitFor(() =>
    expect(confirmReport).toHaveBeenCalledWith(
      expect.objectContaining({ source_ids: [2], content: '完整报告' }),
    ),
  );
});
test('retains the last complete result if regeneration fails', async () => {
  vi.mocked(streamPost)
    .mockImplementationOnce(async (_p, _b, chunk, options) => {
      options?.onSources?.([source]);
      chunk('上次成功报告');
    })
    .mockImplementationOnce(async (_p, _b, chunk) => {
      chunk('未完成片段');
      throw new Error('连接中断');
    });
  page();
  await generate();
  await screen.findByDisplayValue('上次成功报告');
  fireEvent.click(screen.getByText('生成草稿'));
  expect(await screen.findByText('连接中断')).toBeInTheDocument();
  expect(screen.getByDisplayValue('上次成功报告')).toBeInTheDocument();
  fireEvent.click(screen.getByText('确认保存'));
  await waitFor(() =>
    expect(confirmReport).toHaveBeenCalledWith(
      expect.objectContaining({ content: '上次成功报告' }),
    ),
  );
});
test('rejects late preview responses after the user changes report type', async () => {
  let resolve: ((value: Awaited<ReturnType<typeof previewReport>>) => void) | undefined;
  vi.mocked(previewReport).mockImplementation(
    () =>
      new Promise((done) => {
        resolve = done;
      }),
  );
  page();
  fireEvent.click(screen.getByText('选择来源'));
  fireEvent.mouseDown(screen.getAllByRole('combobox')[0]);
  fireEvent.click(await screen.findByText('月报'));
  resolve?.({ sources: [source], start_date: '2026-10-05', end_date: '2026-10-11' });
  await waitFor(() => expect(screen.getByText('生成草稿').closest('button')).toBeDisabled());
  expect(screen.queryByText('原来源')).not.toBeInTheDocument();
});

test('restores a previous successful draft with its own source IDs', async () => {
  vi.mocked(streamPost)
    .mockImplementationOnce(async (_p, _b, chunk, options) => {
      options?.onSources?.([source]);
      chunk('第一份报告');
    })
    .mockImplementationOnce(async (_p, _b, chunk, options) => {
      options?.onSources?.([{ ...source, id: 7 }]);
      chunk('第二份报告');
    });
  page();
  await generate();
  await screen.findByDisplayValue('第一份报告');
  fireEvent.click(screen.getByText('生成草稿'));
  await screen.findByDisplayValue('第二份报告');
  fireEvent.click(screen.getByText('当前标签页生成历史（2）'));
  fireEvent.click(await screen.findByText(/第 1 次成功草稿/));
  fireEvent.click(screen.getByText('恢复为待确认草稿'));
  expect(screen.getByDisplayValue('第一份报告')).toBeInTheDocument();
  fireEvent.click(screen.getByText('确认保存'));
  await waitFor(() =>
    expect(confirmReport).toHaveBeenCalledWith(
      expect.objectContaining({ content: '第一份报告', source_ids: [1] }),
    ),
  );
});
