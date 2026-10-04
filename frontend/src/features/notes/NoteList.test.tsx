import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, expect, test, vi } from 'vitest';
import { createNote, listNotes, searchNotes } from '../../api/notes';
import NoteList from './NoteList';

afterEach(cleanup);
vi.mock('../../api/notes', () => ({
  createNote: vi.fn(),
  listTags: vi.fn().mockResolvedValue([{ id: 4, name: '学习', color: null }]),
  searchNotes: vi.fn().mockResolvedValue({ items: [], total: 0 }),
  listNotes: vi.fn().mockResolvedValue({
    items: [
      {
        id: 1,
        type: 'normal',
        title: '今天的笔记',
        content: '正文',
        note_date: '2026-07-27',
        summary: null,
        word_count: 2,
        created_at: '2026-07-27T00:00:00Z',
        updated_at: '2026-07-27T00:00:00Z',
      },
    ],
    total: 1,
    page: 1,
    page_size: 12,
  }),
}));

test('lists ordinary dated notes and creates a note with today date', async () => {
  vi.mocked(createNote).mockResolvedValue({
    id: 2,
    type: 'normal',
    title: '未命名笔记',
    content: '',
    note_date: '2026-07-27',
    summary: null,
    word_count: 0,
    created_at: '2026-07-27T00:00:00Z',
    updated_at: '2026-07-27T00:00:00Z',
  });

  render(
    <MemoryRouter>
      <QueryClientProvider client={new QueryClient()}>
        <NoteList />
      </QueryClientProvider>
    </MemoryRouter>,
  );

  expect(await screen.findByText('今天的笔记')).toBeInTheDocument();
  expect(screen.getByText('2026-07-27 · 2 字')).toBeInTheDocument();
  expect(screen.queryByPlaceholderText('类型')).not.toBeInTheDocument();
  expect(screen.getByRole('combobox', { name: '筛选标签' })).toBeInTheDocument();
  await waitFor(() =>
    expect(listNotes).toHaveBeenCalledWith(expect.objectContaining({ type: undefined })),
  );

  fireEvent.click(screen.getByRole('button', { name: '新建笔记' }));
  await waitFor(() =>
    expect(createNote).toHaveBeenCalledWith(
      expect.objectContaining({
        type: 'normal',
        note_date: expect.stringMatching(/^\d{4}-\d{2}-\d{2}$/),
      }),
    ),
  );
});

test('passes keyword and selected type to the authenticated search API', async () => {
  render(
    <MemoryRouter>
      <QueryClientProvider client={new QueryClient()}>
        <NoteList />
      </QueryClientProvider>
    </MemoryRouter>,
  );
  fireEvent.mouseDown(screen.getByRole('combobox', { name: '笔记类型' }));
  fireEvent.click(await screen.findByText('周报'));
  fireEvent.change(screen.getByRole('searchbox', { name: '搜索笔记' }), {
    target: { value: '中文学习' },
  });
  fireEvent.keyDown(screen.getByRole('searchbox', { name: '搜索笔记' }), {
    key: 'Enter',
    keyCode: 13,
    charCode: 13,
  });
  await waitFor(() =>
    expect(searchNotes).toHaveBeenCalledWith(
      expect.objectContaining({ q: '中文学习', type: 'weekly' }),
    ),
  );
});
