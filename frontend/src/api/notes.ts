import { http, authHeaders } from './http'
export type Note = { id:number; type:'normal'|'daily'|'weekly'|'monthly'; title:string; content:string; note_date:string|null; summary:string|null; word_count:number; created_at:string; updated_at:string }
export type Tag = { id:number; name:string; color:string|null }
const base = '/api/v1'
export async function listNotes(token:string, params:Record<string,unknown>={}) { return (await http.get<{items:Note[];total:number;page:number;page_size:number}>(`${base}/notes`, {headers:authHeaders(token), params})).data }
export async function getNote(token:string,id:number) { return (await http.get<Note>(`${base}/notes/${id}`, {headers:authHeaders(token)})).data }
export async function createNote(token:string, body:Partial<Note>) { return (await http.post<Note>(`${base}/notes`, body, {headers:authHeaders(token)})).data }
export async function saveNote(token:string,id:number,body:Partial<Note>&{expected_updated_at?:string}) { return (await http.patch<Note>(`${base}/notes/${id}`, body, {headers:authHeaders(token)})).data }
export async function deleteNote(token:string,id:number) { await http.delete(`${base}/notes/${id}`, {headers:authHeaders(token)}) }
export async function listTags(token:string) { return (await http.get<Tag[]>(`${base}/tags`, {headers:authHeaders(token)})).data }
export async function noteTags(token:string,id:number) { return (await http.get<Tag[]>(`${base}/notes/${id}/tags`, {headers:authHeaders(token)})).data }
export async function setNoteTags(token:string,id:number,ids:number[]) { return (await http.put<Tag[]>(`${base}/notes/${id}/tags`, {tag_ids:ids}, {headers:authHeaders(token)})).data }
