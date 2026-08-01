import { render, screen } from '@testing-library/react';
import { expect, test } from 'vitest';
import SafeMarkdown from './SafeMarkdown';

test('drops HTML, unsafe links and remote images', () => {
  const { container } = render(
    <SafeMarkdown>
      {
        '<script>alert(1)</script>\n\n[bad](javascript:evil)\n\n![pixel](https://tracker.invalid/p.png)\n\n[good](https://example.com)'
      }
    </SafeMarkdown>,
  );
  expect(container.querySelector('script')).toBeNull();
  expect(container.querySelector('img')).toBeNull();
  expect(screen.getByText('bad').closest('a')).not.toHaveAttribute('href');
  expect(screen.getByText('good').closest('a')).toHaveAttribute(
    'rel',
    'nofollow noopener noreferrer',
  );
});
