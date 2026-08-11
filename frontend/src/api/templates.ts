import { http } from './http';

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
export async function listPublicTemplates(ranking = 'recommended', cursor = '') {
  return (
    await http.get<{ items: PublicTemplate[]; next_cursor: string }>('/api/v1/templates/public', {
      params: { ranking, cursor: cursor || undefined },
    })
  ).data;
}
export async function listMyTemplates() {
  const response = await http.get<{ items: WritingTemplate[] | null }>('/api/v1/templates/mine');
  return response.data.items ?? [];
}
export async function createTemplate(value: Omit<WritingTemplate, 'id' | 'status' | 'version'>) {
  return (await http.post<WritingTemplate>('/api/v1/templates', value)).data;
}
export async function publishTemplate(id: number) {
  return (await http.post(`/api/v1/templates/${id}/publish`, {})).data;
}
export async function withdrawTemplate(id: number) {
  await http.post(`/api/v1/templates/${id}/withdraw`, {});
}
export async function deleteTemplate(id: number) {
  await http.delete(`/api/v1/templates/${id}`);
}
export async function savePublicProfile(nickname: string) {
  return (await http.put<PublicProfile>('/api/v1/public-profile', { nickname, discoverable: true }))
    .data;
}
export async function useTemplate(id: string) {
  return (
    await http.post<{ note_id: number }>(
      `/api/v1/templates/public/${id}/use`,
      {},
      { headers: { 'Idempotency-Key': crypto.randomUUID() } },
    )
  ).data;
}
export async function usePrivateTemplate(id: number) {
  return (
    await http.post<{ note_id: number }>(
      `/api/v1/templates/${id}/use`,
      {},
      { headers: { 'Idempotency-Key': crypto.randomUUID() } },
    )
  ).data;
}
export async function setTemplateReaction(id: string, kind: 'like' | 'favorite', enabled: boolean) {
  const url = `/api/v1/templates/public/${id}/${kind}`;
  if (enabled) await http.put(url, {});
  else await http.delete(url);
}
export async function recordTemplateView(id: string) {
  await http.post(`/api/v1/templates/public/${id}/views`, {});
}
export async function reportTemplate(id: string, reason: string, details: string) {
  await http.post(`/api/v1/templates/public/${id}/reports`, { reason, details });
}
export async function updateTemplate(value: WritingTemplate) {
  return (
    await http.patch<WritingTemplate>(`/api/v1/templates/${value.id}`, {
      title: value.title,
      description: value.description,
      content_markdown: value.content_markdown,
      category: value.category,
      expected_version: value.version,
    })
  ).data;
}
