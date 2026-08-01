import { authHeaders, http } from './http';

export interface WritingTemplate {
  id: number;
  title: string;
  description: string;
  content_markdown: string;
  category: string;
  status: 'private' | 'published' | 'withdrawn';
  version: number;
  public_id?: string;
}
export interface PublicTemplate {
  public_id: string;
  author_nickname: string;
  version: number;
  title: string;
  description: string;
  content_markdown: string;
  category: string;
  published_at: string;
  like_count: number;
  favorite_count: number;
  usage_count: number;
  liked: boolean;
  favorited: boolean;
}
export interface PublicProfile {
  public_id: string;
  nickname: string;
  discoverable: boolean;
  version: number;
}
const headers = (token: string) => ({ headers: authHeaders(token) });
export async function listPublicTemplates(token: string, ranking = 'recommended', cursor = '') {
  return (
    await http.get<{ items: PublicTemplate[]; next_cursor: string }>('/api/v1/templates/public', {
      ...headers(token),
      params: { ranking, cursor: cursor || undefined },
    })
  ).data;
}
export async function listMyTemplates(token: string) {
  return (await http.get<{ items: WritingTemplate[] }>('/api/v1/templates/mine', headers(token)))
    .data.items;
}
export async function createTemplate(
  token: string,
  value: Omit<WritingTemplate, 'id' | 'status' | 'version'>,
) {
  return (await http.post<WritingTemplate>('/api/v1/templates', value, headers(token))).data;
}
export async function publishTemplate(token: string, id: number) {
  return (await http.post(`/api/v1/templates/${id}/publish`, {}, headers(token))).data;
}
export async function withdrawTemplate(token: string, id: number) {
  await http.post(`/api/v1/templates/${id}/withdraw`, {}, headers(token));
}
export async function deleteTemplate(token: string, id: number) {
  await http.delete(`/api/v1/templates/${id}`, headers(token));
}
export async function savePublicProfile(token: string, nickname: string) {
  return (
    await http.put<PublicProfile>(
      '/api/v1/public-profile',
      { nickname, discoverable: true },
      headers(token),
    )
  ).data;
}
export async function useTemplate(token: string, id: string) {
  return (
    await http.post<{ note_id: number }>(
      `/api/v1/templates/public/${id}/use`,
      {},
      { headers: { ...authHeaders(token), 'Idempotency-Key': crypto.randomUUID() } },
    )
  ).data;
}
export async function usePrivateTemplate(token: string, id: number) {
  return (
    await http.post<{ note_id: number }>(
      `/api/v1/templates/${id}/use`,
      {},
      { headers: { ...authHeaders(token), 'Idempotency-Key': crypto.randomUUID() } },
    )
  ).data;
}
export async function setTemplateReaction(
  token: string,
  id: string,
  kind: 'like' | 'favorite',
  enabled: boolean,
) {
  const url = `/api/v1/templates/public/${id}/${kind}`;
  if (enabled) await http.put(url, {}, headers(token));
  else await http.delete(url, headers(token));
}
export async function recordTemplateView(token: string, id: string) {
  await http.post(`/api/v1/templates/public/${id}/views`, {}, headers(token));
}
export async function reportTemplate(token: string, id: string, reason: string, details: string) {
  await http.post(`/api/v1/templates/public/${id}/reports`, { reason, details }, headers(token));
}
export async function updateTemplate(token: string, value: WritingTemplate) {
  return (
    await http.patch<WritingTemplate>(
      `/api/v1/templates/${value.id}`,
      {
        title: value.title,
        description: value.description,
        content_markdown: value.content_markdown,
        category: value.category,
        expected_version: value.version,
      },
      headers(token),
    )
  ).data;
}
