import { expect, test } from '@playwright/test';

test.skip(!process.env.E2E_REAL_BACKEND, 'requires a real Cortex backend and PostgreSQL');

test('registers, logs in, loads the dashboard, and logs out through the real stack', async ({
  page,
}) => {
  const suffix = `${Date.now()}${Math.floor(Math.random() * 10000)}`;
  const username = `browser_${suffix}`;
  const email = `${username}@example.invalid`;
  const password = 'correct-horse-battery';

  await page.goto('/login');
  await page.getByRole('tab', { name: '注册' }).click();
  let activeForm = page.locator('.ant-tabs-tabpane-active');
  await activeForm.getByPlaceholder('请输入用户名').fill(username);
  await activeForm.getByPlaceholder('请输入邮箱').fill(email);
  await activeForm.getByPlaceholder('请输入密码').fill(password);
  await activeForm.getByRole('button', { name: /注\s*册/ }).click();
  await expect(page.getByText('注册成功，请登录')).toBeVisible();

  activeForm = page.locator('.ant-tabs-tabpane-active');
  await activeForm.getByPlaceholder('请输入用户名').fill(username);
  await activeForm.getByPlaceholder('请输入密码').fill(password);
  await activeForm.getByRole('button', { name: /登\s*录/ }).click();
  await expect(page).toHaveURL(/\/$/);
  await expect(page.getByText(username)).toBeVisible();
  await expect(page.getByRole('heading', { name: '工作台', exact: true })).toBeVisible();

  const eventDialog = page.getByRole('dialog');
  try {
    await eventDialog.waitFor({ state: 'visible', timeout: 2000 });
    await eventDialog.getByRole('button', { name: 'Close' }).click();
  } catch {
    // The event announcement is conditional; absence is a valid dashboard state.
  }
  await page.getByRole('button', { name: '退出登录' }).click();
  await expect(page).toHaveURL(/\/login$/);
});
