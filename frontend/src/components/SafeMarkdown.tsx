import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';

const safeURL = (value?: string) => {
  if (!value) return undefined;
  try {
    const url = new URL(value, window.location.origin);
    return ['http:', 'https:', 'mailto:'].includes(url.protocol) ? value : undefined;
  } catch {
    return undefined;
  }
};

export default function SafeMarkdown({ children }: { children: string }) {
  return (
    <ReactMarkdown
      skipHtml
      remarkPlugins={[remarkGfm]}
      urlTransform={(url) => safeURL(url) || ''}
      components={{
        a: ({ href, children: label }) => (
          <a href={safeURL(href)} rel="nofollow noopener noreferrer" target="_blank">
            {label}
          </a>
        ),
        img: () => null,
      }}
    >
      {children}
    </ReactMarkdown>
  );
}
