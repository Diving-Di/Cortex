import { cleanup, render, screen } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { afterEach, expect, test, vi } from 'vitest';
import ProtectedRoute from './ProtectedRoute';
import { getSession } from '../api/auth';

vi.mock('../api/auth', () => ({ getSession: vi.fn() }));

afterEach(() => cleanup());

function renderRoutes() {
  return render(
    <MemoryRouter initialEntries={['/private']}>
      <Routes>
        <Route path="/login" element={<div>login page</div>} />
        <Route element={<ProtectedRoute />}>
          <Route path="/private" element={<div>private page</div>} />
        </Route>
      </Routes>
    </MemoryRouter>,
  );
}

test('redirects anonymous users to login', async () => {
  vi.mocked(getSession).mockRejectedValueOnce(new Error('unauthorized'));
  renderRoutes();
  expect(await screen.findByText('login page')).toBeInTheDocument();
});

test('renders protected content for authenticated users', async () => {
  vi.mocked(getSession).mockResolvedValueOnce({ username: 'tester' });
  renderRoutes();
  expect(await screen.findByText('private page')).toBeInTheDocument();
});

test('redirects when an authenticated API request later returns 401', async () => {
  vi.mocked(getSession).mockResolvedValueOnce({ username: 'tester' });
  renderRoutes();
  expect(await screen.findByText('private page')).toBeInTheDocument();
  window.dispatchEvent(new Event('auth:unauthorized'));
  expect(await screen.findByText('login page')).toBeInTheDocument();
});
