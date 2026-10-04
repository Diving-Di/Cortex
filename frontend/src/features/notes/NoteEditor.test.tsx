import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { getNote, saveNote, listRevisions, restoreRevision } from '../../api/notes';
import NoteEditor from './NoteEditor';

vi.mock('../../api/notes', () => ({
  getNote: vi.fn(),
  listTags: vi.fn().mockResolvedValue([]),
  noteTags: vi.fn().mockResolvedValue([]),
  setNoteTags: vi.fn(),
  listNotes: vi.fn().mockResolvedValue({ items: [], total: 0 }),
  listRevisions: vi.fn().mockResolvedValue([]),
  restoreRevision: vi.fn(),
  saveNote: vi.fn(),
}));

vi.mock('../../api/http', () => ({
  http: {
    get: vi.fn().mockResolvedValue({ data: [] }),
  },
}));

vi.mock('../../app/theme', () => ({
  useTheme: () => ({ resolved: 'light' }),
}));

vi.mock('@uiw/react-codemirror', () => ({
  default: ({ value, onChange }: { value: string; onChange: (value: string) => void }) => (
    <textarea
      aria-label="笔记正文"
      value={value}
      onChange={(event) => onChange(event.target.value)}
    />
  ),
}));

const originalNote = {
  id: 7,
  type: 'normal' as const,
  title: '原始标题',
  content: '原始正文',
  note_date: '2026-07-28',
  summary: null,
  word_count: 4,
  created_at: '2026-07-28T01:00:00Z',
  updated_at: '2026-07-28T02:00:00Z',
};

function renderEditor() {
  return render(
    <MemoryRouter initialEntries={['/notes/7']}>
      <QueryClientProvider client={new QueryClient()}>
        <Routes>
          <Route path="/notes/:id" element={<NoteEditor />} />
          <Route path="/notes/list" element={<div>笔记本列表</div>} />
        </Routes>
      </QueryClientProvider>
    </MemoryRouter>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  sessionStorage.clear();
  vi.mocked(getNote).mockResolvedValue(originalNote);
});

afterEach(cleanup);

test('does not auto-save and cancel discards edits', async () => {
  renderEditor();

  const title = await screen.findByDisplayValue('原始标题');
  fireEvent.change(title, { target: { value: '未保存标题' } });

  expect(screen.getByText('状态：未保存')).toBeInTheDocument();
  expect(saveNote).not.toHaveBeenCalled();

  fireEvent.click(screen.getByRole('button', { name: /取\s*消/ }));
  expect(await screen.findByText('笔记本列表')).toBeInTheDocument();
  expect(saveNote).not.toHaveBeenCalled();
});

test('waits for the committed save response before returning to the notebook', async () => {
  let finishSave: ((note: typeof originalNote) => void) | undefined;
  vi.mocked(saveNote).mockImplementation(
    () =>
      new Promise((resolve) => {
        finishSave = resolve;
      }),
  );
  renderEditor();

  fireEvent.change(await screen.findByDisplayValue('原始标题'), {
    target: { value: '新标题' },
  });
  fireEvent.click(screen.getByRole('button', { name: /保\s*存/ }));

  expect(screen.getByText('状态：保存中')).toBeInTheDocument();
  expect(screen.queryByText('笔记本列表')).not.toBeInTheDocument();
  expect(saveNote).toHaveBeenCalledWith(7, {
    title: '新标题',
    content: '原始正文',
    note_date: '2026-07-28',
    expected_updated_at: '2026-07-28T02:00:00Z',
  });

  finishSave?.({
    ...originalNote,
    title: '新标题',
    updated_at: '2026-07-28T03:00:00Z',
  });
  await waitFor(() => expect(screen.getByText('笔记本列表')).toBeInTheDocument());
});

test('restores an unsaved draft on refresh without writing to the server', async () => {
  const first = renderEditor();
  fireEvent.change(await screen.findByDisplayValue('原始标题'), {
    target: { value: '本地草稿标题' },
  });
  await waitFor(() => expect(sessionStorage.length).toBe(1));
  first.unmount();
  renderEditor();
  expect(await screen.findByDisplayValue('本地草稿标题')).toBeInTheDocument();
  expect(screen.getByText('已恢复当前标签页的未保存草稿')).toBeInTheDocument();
  expect(saveNote).not.toHaveBeenCalled();
});
test('keeps local content on conflict and requires an explicit merge before retry', async () => {
  vi.mocked(saveNote).mockRejectedValueOnce({ response: { status: 409 } });
  const latest = { ...originalNote, content: '服务器新正文', updated_at: '2026-07-28T04:00:00Z' };
  vi.mocked(getNote).mockResolvedValueOnce(originalNote).mockResolvedValue(latest);
  renderEditor();
  fireEvent.change(await screen.findByDisplayValue('原始正文'), {
    target: { value: '本地新正文' },
  });
  fireEvent.click(screen.getByRole('button', { name: /保\s*存/ }));
  expect(await screen.findByText('服务器新正文')).toBeInTheDocument();
  expect(screen.getByDisplayValue('本地新正文')).toBeInTheDocument();
  expect(screen.getByRole('button', { name: '重试保存' })).toBeDisabled();
  fireEvent.click(screen.getByText('我已合并，使用最新版本'));
  vi.mocked(saveNote).mockResolvedValue({ ...latest, content: '本地新正文' });
  fireEvent.click(screen.getByRole('button', { name: /保\s*存/ }));
  await waitFor(() =>
    expect(saveNote).toHaveBeenLastCalledWith(
      7,
      expect.objectContaining({ content: '本地新正文', expected_updated_at: latest.updated_at }),
    ),
  );
});
test('previews a revision and restores with the displayed note version', async () => {
  vi.spyOn(window, 'confirm').mockReturnValue(true);
  vi.mocked(listRevisions).mockResolvedValue([
    {
      id: 3,
      note_id: 7,
      content: '历史正文',
      reason: 'update',
      created_at: '2026-07-27T00:00:00Z',
    },
  ]);
  vi.mocked(restoreRevision).mockResolvedValue({ ...originalNote, content: '历史正文' });
  renderEditor();
  await screen.findByDisplayValue('原始标题');
  fireEvent.click(screen.getByText('历史版本'));
  fireEvent.click(await screen.findByText(/· update/));
  expect(screen.getByText('历史正文')).toBeInTheDocument();
  expect(restoreRevision).not.toHaveBeenCalled();
  fireEvent.click(screen.getByText('确认恢复正文'));
  await waitFor(() => expect(restoreRevision).toHaveBeenCalledWith(7, 3, originalNote.updated_at));
});
