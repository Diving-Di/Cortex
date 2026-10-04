import { expect, test } from '@playwright/test';

test.skip(!process.env.E2E_REAL_BACKEND, 'requires the isolated Cortex backend');

test('filters real notes, recovers a draft, blocks leaving, and previews/restores history', async ({
  page,
}) => {
  const username = `notes_${Date.now()}_${Math.floor(Math.random() * 1000)}`;
  const password = 'correct-horse-battery';
  const registered = await page.request.post('/api/v1/auth/register', {
    data: { username, email: `${username}@example.invalid`, password },
  });
  expect(registered.ok()).toBeTruthy();
  const loggedIn = await page.request.post('/api/v1/auth/login', { data: { username, password } });
  expect(loggedIn.ok()).toBeTruthy();
  const created = await page.request.post('/api/v1/notes', {
    data: { type: 'normal', title: '浏览器原始标题', content: '原始内容', note_date: '2026-10-05' },
  });
  expect(created.ok()).toBeTruthy();
  const note = await created.json();
  await page.addInitScript(
    (user) => localStorage.setItem(`cortex:guide:${encodeURIComponent(user)}:notes`, 'seen'),
    username,
  );
  await page.goto('/notes');
  await expect(page).toHaveURL(/\/notes\/list$/);
  await page.getByRole('searchbox', { name: '搜索笔记' }).fill('浏览器原始标题');
  await page.getByRole('searchbox', { name: '搜索笔记' }).press('Enter');
  await page.getByText('浏览器原始标题', { exact: true }).click();
  await page.getByRole('textbox', { name: '笔记标题' }).fill('浏览器未提交草稿');
  await expect(page.getByText('状态：未保存')).toBeVisible();
  let allowLeave = false;
  page.on('dialog', async (dialog) => {
    if (dialog.type() === 'beforeunload' || allowLeave) await dialog.accept();
    else await dialog.dismiss();
  });
  await page.getByRole('menuitem', { name: '周期报告' }).click();
  await expect(page).toHaveURL(new RegExp(`/notes/${note.id}$`));
  await page.reload();
  await expect(page.getByText('已恢复当前标签页的未保存草稿')).toBeVisible();
  await expect(page.getByRole('textbox', { name: '笔记标题' })).toHaveValue('浏览器未提交草稿');
  const stillOriginal = await page.request.get(`/api/v1/notes/${note.id}`);
  expect((await stillOriginal.json()).title).toBe('浏览器原始标题');
  await page.locator('.cm-content').fill('浏览器新正文');
  await page.getByRole('button', { name: /^保\s*存$/ }).click();
  await expect(page).toHaveURL(/\/notes\/list$/);
  await page.getByText('浏览器未提交草稿', { exact: true }).click();
  await page.getByRole('button', { name: '历史版本' }).click();
  await page
    .getByRole('dialog', { name: '历史版本' })
    .getByRole('button', { name: /· update/ })
    .first()
    .click();
  await expect(
    page.getByRole('dialog', { name: '历史版本' }).getByText('原始内容', { exact: true }),
  ).toBeVisible();
  allowLeave = true;
  await page.getByRole('button', { name: '确认恢复正文' }).click();
  await expect(page.locator('.cm-content')).toHaveText('原始内容');
  const restored = await page.request.get(`/api/v1/notes/${note.id}`);
  expect((await restored.json()).content).toBe('原始内容');
});
