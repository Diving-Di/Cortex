import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, expect, test, vi } from 'vitest';
import DashboardPage from './DashboardPage';

vi.mock('../../api/dashboard', () => ({
  getDashboard: vi.fn().mockResolvedValue({
    date: '2026-08-01',
    today: { new_notes: 0 },
    streak_days: 0,
    statistics: { notes: 0, ai_tokens: 0 },
    activity: [],
    recent_notes: [],
    pending_reports: [],
  }),
}));
vi.mock('../../api/aiEvents', () => ({
  getCurrentAIEvent: vi.fn().mockResolvedValue({
    id: 'event-1',
    timezone: 'Asia/Shanghai',
    opens_at: '2026-08-01T12:30:00Z',
    closes_at: '2026-08-01T12:42:00Z',
    total_slots: 7,
    points_reward: 80,
    required_streak_days: 4,
    show_dashboard_prompt: true,
  }),
}));
vi.mock('../../api/m2', () => ({ confirmOrganize: vi.fn(), streamPost: vi.fn() }));

beforeEach(() => localStorage.clear());

function renderDashboard() {
  return render(
    <QueryClientProvider client={new QueryClient()}>
      <MemoryRouter>
        <DashboardPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

vi.mock('./PendingWork', () => ({ default: () => <div>待处理任务</div> }));

test('shows an optional event card without interrupting the workbench', async () => {
  renderDashboard();
  expect(await screen.findByText('20:30 开放 · 12 分钟 · 7 个名额 · 80 点')).toBeInTheDocument();
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  expect(screen.getByRole('link', { name: '查看活动' })).toHaveAttribute('href', '/ai-events');
});
