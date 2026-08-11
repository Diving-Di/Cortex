import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import type { Preferences } from '../../api/settings';
import SettingsPage from './SettingsPage';

const updatePreferences = vi.fn(async (value: Preferences) => ({
  ...value,
  version: value.version + 1,
}));

vi.mock('../../app/theme', () => ({
  useTheme: () => ({ preference: 'system', setPreference: vi.fn() }),
}));

vi.mock('../../api/settings', () => ({
  getPreferences: vi.fn(async () => ({
    marketplace_personalization: true,
    version: 1,
  })),
  updatePreferences: (value: Preferences) => updatePreferences(value),
}));

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

test('toggles marketplace personalization from settings', async () => {
  render(<SettingsPage />);

  const toggle = await screen.findByLabelText('个性化模板推荐');
  expect(toggle).toBeChecked();
  fireEvent.click(toggle);

  await waitFor(() =>
    expect(updatePreferences).toHaveBeenCalledWith({
      marketplace_personalization: false,
      version: 1,
    }),
  );
});
