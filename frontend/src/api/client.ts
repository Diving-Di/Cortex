export type ApiErrorPayload = {
  code?: string;
  message?: string;
  details?: unknown;
  error?: { message?: string };
  detail?: { message?: string };
};

export class ApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
    readonly code = 'REQUEST_FAILED',
    readonly details?: unknown,
  ) {
    super(message);
    this.name = 'ApiError';
  }
}

export async function errorFromResponse(response: Response): Promise<ApiError> {
  const payload = (await response.json().catch(() => ({}))) as ApiErrorPayload;
  return new ApiError(
    payload.message ||
      payload.error?.message ||
      payload.detail?.message ||
      `HTTP ${response.status}`,
    response.status,
    payload.code,
    payload.details,
  );
}

export type SSEMessage = { event: string; data: string };

function parseBlock(block: string): SSEMessage | undefined {
  let event = 'message';
  const data: string[] = [];
  for (const line of block.split(/\r?\n/)) {
    if (line.startsWith('event:')) event = line.slice(6).trim();
    if (line.startsWith('data:')) data.push(line.slice(5).trimStart());
  }
  if (!data.length) return undefined;
  return { event, data: data.join('\n') };
}

export async function consumeSSE(
  response: Response,
  onMessage: (message: SSEMessage) => void,
): Promise<void> {
  if (!response.body)
    throw new ApiError('浏览器不支持流式响应', response.status, 'STREAM_UNSUPPORTED');
  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = '';
  while (true) {
    const { done, value } = await reader.read();
    buffer += decoder.decode(value, { stream: !done });
    const blocks = buffer.split(/\r?\n\r?\n/);
    buffer = blocks.pop() ?? '';
    for (const block of blocks) {
      const message = parseBlock(block);
      if (message) onMessage(message);
    }
    if (done) break;
  }
  if (buffer.trim()) {
    const message = parseBlock(buffer);
    if (message) onMessage(message);
  }
}
