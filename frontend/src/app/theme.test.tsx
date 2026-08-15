import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ThemeProvider, useTheme } from './theme';

let systemDark = false;
const listeners = new Set<() => void>();

beforeEach(() => {
  cleanup();
  localStorage.clear();
  systemDark = false;
  listeners.clear();
  vi.stubGlobal('matchMedia', () => ({
    matches: systemDark,
    media: '(prefers-color-scheme: dark)',
    onchange: null,
    addEventListener: (_: string, listener: () => void) => listeners.add(listener),
    removeEventListener: (_: string, listener: () => void) => listeners.delete(listener),
    addListener: vi.fn(),
    removeListener: vi.fn(),
    dispatchEvent: vi.fn(),
  }));
});

function Controls() {
  const { preference, resolved, setPreference } = useTheme();
  return (
    <>
      <span>{`${preference}:${resolved}`}</span>
      <button onClick={() => setPreference('dark')}>dark</button>
      <button onClick={() => setPreference('system')}>system</button>
    </>
  );
}

describe('ThemeProvider', () => {
  it('persists an explicit theme and applies it to the document', () => {
    render(
      <ThemeProvider>
        <Controls />
      </ThemeProvider>,
    );
    fireEvent.click(screen.getByText('dark'));
    expect(screen.getByText('dark:dark')).toBeInTheDocument();
    expect(localStorage.getItem('cortex.theme')).toBe('dark');
    expect(document.documentElement.dataset.theme).toBe('dark');
  });

  it('follows system theme changes', () => {
    render(
      <ThemeProvider>
        <Controls />
      </ThemeProvider>,
    );
    fireEvent.click(screen.getByText('system'));
    systemDark = true;
    act(() => listeners.forEach((listener) => listener()));
    expect(screen.getByText('system:dark')).toBeInTheDocument();
  });

  it('migrates the legacy stored theme', () => {
    localStorage.setItem('diary-listener.theme', 'dark');
    render(
      <ThemeProvider>
        <Controls />
      </ThemeProvider>,
    );
    expect(screen.getByText('dark:dark')).toBeInTheDocument();
    expect(localStorage.getItem('cortex.theme')).toBe('dark');
  });

  it('falls back to system for an invalid stored value', () => {
    localStorage.setItem('diary-listener.theme', 'invalid');
    render(
      <ThemeProvider>
        <Controls />
      </ThemeProvider>,
    );
    expect(screen.getByText('system:light')).toBeInTheDocument();
  });
});
