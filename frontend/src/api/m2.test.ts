import { describe, expect, it, vi } from 'vitest';
import { IncompleteStreamError, streamPost } from './m2';

function streamResponse(chunks: string[]) {
  const encoder = new TextEncoder();
  return new Response(
    new ReadableStream({
      start(controller) {
        chunks.forEach((chunk) => controller.enqueue(encoder.encode(chunk)));
        controller.close();
      },
    }),
    { status: 200, headers: { 'Content-Type': 'text/event-stream' } },
  );
}

describe('streamPost', () => {
  it('rejects a partial stream that ends without DONE', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => streamResponse(['data: {"content":"partial"}\n\n'])),
    );
    const chunks: string[] = [];

    await expect(
      streamPost('/ai/organize', {}, (chunk) => chunks.push(chunk)),
    ).rejects.toBeInstanceOf(IncompleteStreamError);
    expect(chunks).toEqual(['partial']);
    vi.unstubAllGlobals();
  });

  it('accepts a stream only after DONE', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => streamResponse(['data: {"content":"ok"}\n\ndata: [DONE]\n\n'])),
    );
    const chunks: string[] = [];

    await streamPost('/ai/organize', {}, (chunk) => chunks.push(chunk));
    expect(chunks).toEqual(['ok']);
    vi.unstubAllGlobals();
  });
});
