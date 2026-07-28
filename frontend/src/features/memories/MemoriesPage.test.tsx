import { render, screen } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { vi } from 'vitest';
import MemoriesPage from './MemoriesPage';

vi.mock('../../api/memories', () => ({
  listMemories: vi.fn().mockResolvedValue({ items: [], total: 0 }),
  getMemorySettings: vi.fn().mockResolvedValue({
    suggestion_enabled: false,
    allowed_categories: ['fact', 'preference', 'goal', 'habit', 'milestone'],
    minimum_importance: 5,
    excluded_note_types: [],
    excluded_tag_ids: [],
  }),
  createMemory: vi.fn(),
  updateMemory: vi.fn(),
  deleteMemory: vi.fn(),
  saveMemorySettings: vi.fn(),
  createMemoryDraft: vi.fn(),
  confirmMemoryDraft: vi.fn(),
  rejectMemoryDraft: vi.fn(),
}));

test('renders manual memories and confirmed AI suggestion controls', async () => {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <MemoriesPage token="token" />
    </QueryClientProvider>,
  );
  expect(await screen.findByText('成长记忆')).toBeInTheDocument();
  expect(screen.getByText('AI 记忆建议（确认后才保存）')).toBeInTheDocument();
  expect(screen.getByRole('button', { name: '生成建议' })).toBeDisabled();
});
