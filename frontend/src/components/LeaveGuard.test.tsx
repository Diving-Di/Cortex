import { fireEvent, render, screen, waitFor, cleanup } from '@testing-library/react';
import { createMemoryRouter, RouterProvider, Link } from 'react-router-dom';
import { afterEach, expect, test, vi } from 'vitest';
import LeaveGuard from './LeaveGuard';
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});
test('blocks in-app navigation until the user accepts leaving an unsaved draft', async () => {
  const NativeRequest = globalThis.Request;
  vi.stubGlobal(
    'Request',
    class extends NativeRequest {
      constructor(input: RequestInfo | URL, init?: RequestInit) {
        super(input, { ...init, signal: undefined });
      }
    },
  );
  const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false);
  const router = createMemoryRouter([
    {
      path: '/',
      element: (
        <>
          <LeaveGuard dirty />
          <Link to="/next">离开编辑</Link>
        </>
      ),
    },
    { path: '/next', element: <p>下一页</p> },
  ]);
  render(<RouterProvider router={router} />);
  fireEvent.click(screen.getByText('离开编辑'));
  await waitFor(() => expect(confirm).toHaveBeenCalledOnce());
  expect(screen.queryByText('下一页')).not.toBeInTheDocument();
  confirm.mockReturnValue(true);
  fireEvent.click(screen.getByText('离开编辑'));
  expect(await screen.findByText('下一页')).toBeInTheDocument();
});
