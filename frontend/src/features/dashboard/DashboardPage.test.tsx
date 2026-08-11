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

test('uses server event configuration and remembers dismissal', async () => {
  const first = renderDashboard();
  expect(await screen.findByText('今晚 20:30 免费点数限量开放')).toBeInTheDocument();
  expect(screen.getByText(/持续 12 分钟，共 7 个名额，成功领取可获得 80 点/)).toBeInTheDocument();
  fireEvent.click(screen.getByText('今日不再提醒'));
  expect(localStorage.getItem('ai-event-modal-dismissed:event-1')).toBe('1');
  first.unmount();
  renderDashboard();
  await waitFor(() =>
    expect(screen.queryByText('今晚 20:30 免费点数限量开放')).not.toBeInTheDocument(),
  );
});
