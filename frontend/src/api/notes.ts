import { http } from './http';
export type Note = {
  id: number;
  type: 'normal' | 'daily' | 'weekly' | 'monthly';
  title: string;
  content: string;
  note_date: string | null;
  summary: string | null;
  word_count: number;
  created_at: string;
  updated_at: string;
};
export type Tag = { id: number; name: string; color: string | null };
const base = '/api/v1';
export async function listNotes(params: Record<string, unknown> = {}) {
  return (
    await http.get<{ items: Note[]; total: number; page: number; page_size: number }>(
      `${base}/notes`,
      { params },
    )
  ).data;
}
export async function getNote(id: number) {
  return (await http.get<Note>(`${base}/notes/${id}`)).data;
}
export async function createNote(body: Partial<Note>) {
  return (await http.post<Note>(`${base}/notes`, body)).data;
}
export async function saveNote(id: number, body: Partial<Note> & { expected_updated_at?: string }) {
  return (await http.patch<Note>(`${base}/notes/${id}`, body)).data;
}
export async function deleteNote(id: number) {
  await http.delete(`${base}/notes/${id}`);
}
export async function listTags() {
  return (await http.get<Tag[]>(`${base}/tags`)).data;
}
export async function noteTags(id: number) {
  return (await http.get<Tag[]>(`${base}/notes/${id}/tags`)).data;
}
export async function setNoteTags(id: number, ids: number[]) {
  return (await http.put<Tag[]>(`${base}/notes/${id}/tags`, { tag_ids: ids })).data;
}
