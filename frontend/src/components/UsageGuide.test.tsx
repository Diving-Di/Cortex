import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, expect, test } from 'vitest';
import UsageGuide from './UsageGuide';
beforeEach(() => localStorage.clear());
afterEach(cleanup);
test('can skip the first visit guide and reopen it explicitly', () => {
  const steps = [{ title: '记录', description: '创建笔记' }];
  const first = render(<UsageGuide id="example" steps={steps} />);
  expect(screen.getByRole('dialog')).toBeInTheDocument();
  fireEvent.click(screen.getByRole('button', { name: /跳\s*过/ }));
  first.unmount();
  render(<UsageGuide id="example" steps={steps} />);
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  fireEvent.click(screen.getByText('使用引导'));
  expect(screen.getByRole('dialog')).toBeInTheDocument();
});
