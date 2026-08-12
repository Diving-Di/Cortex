import { expect, test } from 'vitest';

test('supports computed-style calls with a pseudo-element argument', () => {
  const element = document.createElement('div');
  element.style.display = 'block';
  document.body.appendChild(element);

  expect(window.getComputedStyle(element, '::-webkit-scrollbar').display).toBe('block');

  element.remove();
});
