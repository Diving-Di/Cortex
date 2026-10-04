import { beforeEach, expect, test, vi } from 'vitest';
import { listDrafts, readDraft, removeDraft, writeDraft } from './drafts';
beforeEach(() => {
  sessionStorage.clear();
  vi.restoreAllMocks();
});
test('isolates accounts and removes only the saved draft', () => {
  writeDraft('alice', {
    key: 'note:1',
    label: 'a',
    path: '/notes/1',
    value: { content: 'private' },
  });
  writeDraft('bob', { key: 'note:1', label: 'b', path: '/notes/1', value: { content: 'other' } });
  expect(listDrafts('alice')).toHaveLength(1);
  expect(readDraft<{ content: string }>('bob', 'note:1')?.value.content).toBe('other');
  removeDraft('alice', 'note:1');
  expect(listDrafts('alice')).toEqual([]);
  expect(listDrafts('bob')).toHaveLength(1);
});
test('expires stale drafts and tolerates broken storage', () => {
  writeDraft('alice', { key: 'x', label: 'x', path: '/', value: 'text' });
  vi.spyOn(Date, 'now').mockReturnValue(Date.now() + 8 * 24 * 60 * 60 * 1000);
  expect(readDraft('alice', 'x')).toBeUndefined();
  vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
    throw new Error('quota');
  });
  expect(writeDraft('alice', { key: 'x', label: 'x', path: '/', value: 'text' })).toBe(false);
});
