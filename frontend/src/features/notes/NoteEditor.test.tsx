import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { getNote, saveNote } from '../../api/notes';
import NoteEditor from './NoteEditor';

vi.mock('../../api/notes', () => ({
  getNote: vi.fn(),
  saveNote: vi.fn(),
}));

vi.mock('../../api/http', () => ({
  authHeaders: () => ({}),
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
          <Route path="/notes/:id" element={<NoteEditor token="test-token" />} />
          <Route path="/notes/list" element={<div>笔记本列表</div>} />
        </Routes>
      </QueryClientProvider>
    </MemoryRouter>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
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
  expect(saveNote).toHaveBeenCalledWith('test-token', 7, {
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
