import type { DiaryEntry } from '../types'
import { authHeaders, http } from './http'
import { apiUrls } from './urls'

export async function fetchDiaryEntries(token: string): Promise<DiaryEntry[]> {
  const res = await http.get<DiaryEntry[]>(apiUrls.diary.list, {
    headers: authHeaders(token),
  })
  return res.data || []
}

export async function createDiaryEntry(
  token: string,
  content: string,
  image?: File | null,
): Promise<DiaryEntry> {
  const form = new FormData()
  form.append('content', content)
  if (image) {
    form.append('image', image)
  }
  const res = await http.post<DiaryEntry>(apiUrls.diary.create, form, {
    headers: authHeaders(token),
  })
  return res.data
}

export async function deleteDiaryEntry(token: string, id: number): Promise<void> {
  await http.delete(apiUrls.diary.detail(id), { headers: authHeaders(token) })
}
