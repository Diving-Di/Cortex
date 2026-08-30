import { expect, test } from '@playwright/test';

test('logs in through the browser contract and reaches the protected dashboard', async ({
  page,
}) => {
  let authenticated = false;
  await page.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    if (url.pathname === '/api/v1/auth/session') {
      await route.fulfill(
        authenticated
          ? {
              status: 200,
              contentType: 'application/json',
              body: JSON.stringify({ username: 'e2e-user' }),
            }
          : {
              status: 401,
              contentType: 'application/json',
              body: JSON.stringify({
                code: 'AUTHENTICATION_REQUIRED',
                message: 'sign in',
                details: null,
              }),
            },
      );
      return;
    }
    if (url.pathname === '/api/v1/auth/login' && request.method() === 'POST') {
      authenticated = true;
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ username: 'e2e-user' }),
      });
      return;
    }
    if (url.pathname === '/api/v1/dashboard') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          date: '2026-08-29',
          timezone: 'Asia/Shanghai',
          today: { new_notes: 0 },
          streak_days: 0,
          statistics: { notes: 0, words: 0, ai_requests: 0, ai_tokens: 0 },
          recent_notes: [],
          activity: [],
          pending_reports: [],
        }),
      });
      return;
    }
    if (url.pathname === '/api/v1/ai-events/current') {
      await route.fulfill({
        status: 404,
        contentType: 'application/json',
        body: JSON.stringify({ code: 'NOT_FOUND', message: 'none', details: null }),
      });
      return;
    }
    await route.fulfill({ status: 404, contentType: 'application/json', body: '{}' });
  });

  await page.goto('/');
  await expect(page).toHaveURL(/\/login$/);
  await page.getByPlaceholder('请输入用户名').fill('e2e-user');
  await page.getByPlaceholder('请输入密码').fill('correct horse battery staple');
  await page.getByRole('button', { name: /登\s*录/ }).click();

  await expect(page).toHaveURL(/\/$/);
  await expect(page.getByText('e2e-user')).toBeVisible();
  await expect(page.getByText('工作台', { exact: true })).toBeVisible();
});
