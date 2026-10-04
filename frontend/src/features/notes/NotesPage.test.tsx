import { cleanup, render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import NotesPage from './NotesPage';

vi.mock('../templates/TemplatesPage', () => ({ default: () => <div>模板广场内容</div> }));
vi.mock('./NoteList', () => ({ default: () => <div>我的笔记内容</div> }));
vi.mock('./NoteEditor', () => ({ default: () => <div>笔记编辑器</div> }));

beforeEach(() => localStorage.clear());
afterEach(cleanup);

test('opens notes by default', () => {
  render(
    <MemoryRouter initialEntries={['/']}>
      <NotesPage />
    </MemoryRouter>,
  );
  expect(screen.getByText('我的笔记内容')).toBeInTheDocument();
});

test('restores the latest notes section choice', async () => {
  localStorage.setItem('cortex:notes-section', 'list');
  render(
    <MemoryRouter initialEntries={['/']}>
      <NotesPage />
    </MemoryRouter>,
  );
  expect(await screen.findByText('我的笔记内容')).toBeInTheDocument();
});

test('exposes templates on an explicit route', () => {
  render(
    <MemoryRouter initialEntries={['/templates']}>
      <NotesPage />
    </MemoryRouter>,
  );
  expect(screen.getByText('模板广场内容')).toBeInTheDocument();
});
