import { authHeaders, http } from './http';
export type Source = { id: number; title: string; note_date: string | null; snippet: string };
export async function streamPost(
  token: string,
  path: string,
  body: unknown,
  onChunk: (text: string) => void,
) {
  const response = await fetch(`/api/v1${path}`, {
    method: 'POST',
    headers: { ...authHeaders(token), 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
  if (!response.ok) {
    const data = await response.json().catch(() => ({ message: '请求失败' }));
    throw new Error(data.message || data.error?.message || data.detail?.message || '请求失败');
  }
  const reader = response.body?.getReader();
  if (!reader) return;
  const decoder = new TextDecoder();
  let buffer = '';
  while (true) {
    const { done, value } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });
    const events = buffer.split('\n\n');
    buffer = events.pop() || '';
    for (const event of events) {
      const line = event.split('\n').find((x) => x.startsWith('data: '));
      if (!line) continue;
      const raw = line.slice(6);
      if (raw === '[DONE]') continue;
      const data = JSON.parse(raw);
      if (data.content) onChunk(data.content);
    }
  }
}
export async function confirmOrganize(token: string, body: unknown) {
  return (await http.post('/api/v1/ai/organize/confirm', body, { headers: authHeaders(token) }))
    .data;
}
export async function previewReport(token: string, body: unknown) {
  return (
    await http.post<{ start_date: string; end_date: string; sources: Source[] }>(
      '/api/v1/reports/preview',
      body,
      { headers: authHeaders(token) },
    )
  ).data;
}
export async function confirmReport(token: string, body: unknown) {
  return (await http.post('/api/v1/reports/confirm', body, { headers: authHeaders(token) })).data;
}
