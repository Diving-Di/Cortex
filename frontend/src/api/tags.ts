import type { TagRecord } from '../types/image'
import { normalizeTagNames } from '../utils/tags'
import { API_BASE_URL } from './config'
import { authHeaders, http } from './http'

export async function fetchCustomTagNames(token: string): Promise<string[]> {
  const res = await http.get<TagRecord[]>(`${API_BASE_URL}/tags/`, {
    headers: authHeaders(token),
  })
  const names = (res.data || [])
    .filter(tag => tag && tag.tag_type === 'Custom')
    .flatMap(tag => normalizeTagNames(tag.tag_name || ''))

  return Array.from(new Set(names))
}
