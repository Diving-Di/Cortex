import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import AIEventsPage from './AIEventsPage';
import { getCurrentAIEvent } from '../../api/aiEvents';

const state = vi.hoisted(() => ({
  event: {} as any,
  balance: {} as any,
}));
vi.mock('../../api/aiEvents', () => ({
  getCurrentAIEvent: vi.fn(() => Promise.resolve({ ...state.event })),
  getAIPointBalance: vi.fn(() => Promise.resolve({ ...state.balance })),
  claimAIEvent: vi.fn(),
  getMyAIEventClaim: vi.fn().mockResolvedValue(undefined),
  getAIEventHistory: vi
    .fn()
    .mockResolvedValue([{ display_name: '记录者·A1B2', claimed_at: '2026-08-01T12:00:00Z' }]),
}));

beforeEach(() => {
  Object.assign(state.event, {
    id: 'e',
    event_date: '2026-08-01',
    timezone: 'Asia/Shanghai',
    opens_at: new Date(Date.now() - 1000).toISOString(),
    closes_at: new Date(Date.now() + 600000).toISOString(),
    total_slots: 10,
    remaining_slots: 8,
    points_cost: 100,
    required_streak_days: 5,
    status: 'open',
    server_time: new Date().toISOString(),
    eligible: true,
    streak_days: 5,
    claimed: false,
  });
  Object.assign(state.balance, {
    period_start: '2026-08-01',
    granted: 1000,
    consumed: 0,
    held: 0,
    available: 1000,
    version: 1,
  });
  vi.mocked(getCurrentAIEvent).mockClear();
});
afterEach(cleanup);

function renderPage() {
  return render(
    <QueryClientProvider client={new QueryClient()}>
      <MemoryRouter>
        <AIEventsPage token="t" />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

test('shows configured event and eligibility', async () => {
  renderPage();
  expect(await screen.findByText('每日限量 AI 深度月报')).toBeInTheDocument();
  expect(screen.getByText('立即领取')).toBeEnabled();
  expect(screen.getByText('100')).toBeInTheDocument();
  expect(await screen.findByText('记录者·A1B2')).toBeInTheDocument();
});

test('shows sold-out and insufficient-points reasons', async () => {
  state.event.remaining_slots = 0;
  state.balance.available = 20;
  renderPage();
  expect(await screen.findByText('本场名额已领完')).toBeInTheDocument();
  expect(screen.getByText('AI 点数不足')).toBeInTheDocument();
  expect(screen.getByRole('button', { name: '立即领取' })).toBeDisabled();
});

test('resynchronizes server time when the page becomes visible', async () => {
  renderPage();
  await screen.findByText('每日限量 AI 深度月报');
  const initialCalls = vi.mocked(getCurrentAIEvent).mock.calls.length;
  Object.defineProperty(document, 'visibilityState', { configurable: true, value: 'visible' });
  fireEvent(document, new Event('visibilitychange'));
  await waitFor(() =>
    expect(vi.mocked(getCurrentAIEvent).mock.calls.length).toBeGreaterThan(initialCalls),
  );
});
