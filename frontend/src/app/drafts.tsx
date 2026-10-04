import { createContext, useContext } from 'react';

export const DraftScope = createContext('test-session');
export const useDraftScope = () => useContext(DraftScope);
export type Draft<T = unknown> = {
  key: string;
  label: string;
  path: string;
  value: T;
  savedAt: number;
};
const prefix = (scope: string) => `cortex:draft:${encodeURIComponent(scope)}:`;
const ttl = 7 * 24 * 60 * 60 * 1000;

export function readDraft<T>(scope: string, key: string): Draft<T> | undefined {
  try {
    const raw = sessionStorage.getItem(prefix(scope) + key);
    if (!raw) return;
    const draft = JSON.parse(raw) as Draft<T>;
    if (!draft || typeof draft.savedAt !== 'number' || Date.now() - draft.savedAt > ttl) {
      sessionStorage.removeItem(prefix(scope) + key);
      return;
    }
    return draft;
  } catch {
    return;
  }
}
export function writeDraft<T>(scope: string, draft: Omit<Draft<T>, 'savedAt'>) {
  try {
    sessionStorage.setItem(
      prefix(scope) + draft.key,
      JSON.stringify({ ...draft, savedAt: Date.now() }),
    );
    window.dispatchEvent(new Event('cortex:drafts'));
    return true;
  } catch {
    return false;
  }
}
export function removeDraft(scope: string, key: string) {
  try {
    sessionStorage.removeItem(prefix(scope) + key);
  } catch {
    /* Storage may be disabled. */
  }
  window.dispatchEvent(new Event('cortex:drafts'));
}
export function listDrafts(scope: string): Draft[] {
  try {
    return Object.keys(sessionStorage)
      .filter((key) => key.startsWith(prefix(scope)))
      .map((key) => readDraft(scope, key.slice(prefix(scope).length)))
      .filter((item): item is Draft => Boolean(item));
  } catch {
    return [];
  }
}
