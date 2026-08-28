import { http } from './http';
import { consumeSSE, errorFromResponse } from './client';
export type Source = { id: number; title: string; note_date: string | null; snippet: string };
export class IncompleteStreamError extends Error {
  readonly incomplete = true;
  constructor(
    message: string,
    readonly outputTokens = 0,
    readonly upstreamStage = 'generation',
  ) {
    super(message);
    this.name = 'IncompleteStreamError';
  }
}
export async function streamPost(path: string, body: unknown, onChunk: (text: string) => void) {
  const response = await fetch(`/api/v1${path}`, {
    method: 'POST',
    credentials: 'same-origin',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
  if (!response.ok) {
    throw await errorFromResponse(response);
  }
  let receivedDone = false;
  await consumeSSE(response, ({ event, data: raw }) => {
    if (raw === '[DONE]') {
      receivedDone = true;
      return;
    }
    const data = JSON.parse(raw);
    if (event === 'error') {
      throw new IncompleteStreamError(
        data.message || '生成中断，已保留未完成内容',
        data.output_tokens || 0,
        data.upstream_stage || 'generation',
      );
    }
    if (data.content) onChunk(data.content);
  });
  if (!receivedDone) {
    throw new IncompleteStreamError('连接提前结束，回答未完成');
  }
}
export async function confirmOrganize(body: unknown) {
  return (await http.post('/api/v1/ai/organize/confirm', body)).data;
}
export async function previewReport(body: unknown) {
  return (
    await http.post<{ start_date: string; end_date: string; sources: Source[] }>(
      '/api/v1/reports/preview',
      body,
    )
  ).data;
}
export async function confirmReport(body: unknown) {
  return (await http.post('/api/v1/reports/confirm', body)).data;
}
