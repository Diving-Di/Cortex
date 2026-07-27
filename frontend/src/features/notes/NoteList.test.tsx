import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { expect, test, vi } from 'vitest';
import { createNote, listNotes } from '../../api/notes';
import NoteList from './NoteList';

vi.mock('../../api/notes', () => ({
  createNote: vi.fn(),
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
        <NoteList token="test-token" />
      </QueryClientProvider>
    </MemoryRouter>,
  );

  expect(await screen.findByText('今天的笔记')).toBeInTheDocument();
  expect(screen.getByText('2026-07-27 · 2 字')).toBeInTheDocument();
  expect(screen.queryByPlaceholderText('类型')).not.toBeInTheDocument();
  expect(screen.queryByPlaceholderText('标签')).not.toBeInTheDocument();
  await waitFor(() =>
    expect(listNotes).toHaveBeenCalledWith(
      'test-token',
      expect.objectContaining({ type: 'normal' }),
    ),
  );

  fireEvent.click(screen.getByRole('button', { name: '新建笔记' }));
  await waitFor(() =>
    expect(createNote).toHaveBeenCalledWith(
      'test-token',
      expect.objectContaining({
        type: 'normal',
        note_date: expect.stringMatching(/^\d{4}-\d{2}-\d{2}$/),
      }),
    ),
  );
});
