import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import type { RecipePreferences } from '../../api/recipes';
import SettingsPage from './SettingsPage';

const updateRecipePreferences = vi.fn(async (_token: string, value: RecipePreferences) => ({
  ...value,
  version: value.version + 1,
}));

vi.mock('../../app/theme', () => ({
  useTheme: () => ({ preference: 'system', setPreference: vi.fn() }),
}));

vi.mock('../../api/recipes', () => ({
  getRecipePreferences: vi.fn(async () => ({
    dietary_restrictions: ['花生'],
    timezone: 'Asia/Shanghai',
    marketplace_personalization: true,
    version: 1,
  })),
  updateRecipePreferences: (...args: [string, RecipePreferences]) =>
    updateRecipePreferences(...args),
}));

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

test('edits and saves dietary restrictions from settings', async () => {
  render(<SettingsPage token="token" />);

  const input = await screen.findByLabelText('忌口食材');
  expect(input).toHaveValue('花生');
  fireEvent.change(input, { target: { value: '花生，香菜' } });
  fireEvent.click(screen.getByRole('button', { name: /保\s*存/ }));

  await waitFor(() =>
    expect(updateRecipePreferences).toHaveBeenCalledWith('token', {
      dietary_restrictions: ['花生', '香菜'],
      timezone: 'Asia/Shanghai',
      marketplace_personalization: true,
      version: 1,
    }),
  );
  expect(screen.getByText(/作为菜谱问答的系统约束/)).toBeInTheDocument();
});
