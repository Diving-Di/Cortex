import '@testing-library/jest-dom/vitest';

// Ant Design uses the pseudo-element argument while measuring scrollbars.
// jsdom does not implement that argument and logs a noisy "Not implemented"
// error, so keep its normal computed-style behavior and ignore only the
// unsupported argument in tests.
const getComputedStyle = window.getComputedStyle.bind(window);
Object.defineProperty(window, 'getComputedStyle', {
  configurable: true,
  value: (element: Element) => getComputedStyle(element),
});

Object.defineProperty(window, 'matchMedia', {
  writable: true,
  value: (query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: () => undefined,
    removeListener: () => undefined,
    addEventListener: () => undefined,
    removeEventListener: () => undefined,
    dispatchEvent: () => false,
  }),
});
