import type { ImageId, ImageItem } from '../types/image'
import { API_BASE_URL } from './config'
import { authHeaders, http } from './http'

export async function fetchImages(token: string, filterTag?: string): Promise<ImageItem[]> {
  const params: { tag?: string } = {}
  if (filterTag && String(filterTag).trim()) {
    params.tag = String(filterTag).trim()
  }

  const res = await http.get<ImageItem[]>(`${API_BASE_URL}/images/`, {
    headers: authHeaders(token),
    params,
  })
  return res.data || []
}

export async function uploadImage(token: string, file: Blob, tagNames: string[]): Promise<void> {
  const form = new FormData()
  form.append('image', file)
  form.append('tag_names', JSON.stringify(tagNames))

  await http.post(`${API_BASE_URL}/images/`, form, {
    headers: {
      'Content-Type': 'multipart/form-data',
      ...authHeaders(token),
    },
  })
}

export async function updateImageTags(token: string, imageId: ImageId, tagNames: string[]): Promise<void> {
  const form = new FormData()
  form.append('tag_names', JSON.stringify(tagNames))

  await http.patch(`${API_BASE_URL}/images/${imageId}/`, form, {
    headers: authHeaders(token),
  })
}

export async function deleteImage(token: string, imageId: ImageId): Promise<void> {
  await http.delete(`${API_BASE_URL}/images/${imageId}/`, {
    headers: authHeaders(token),
  })
}

export async function generateAiTags(token: string, imageId: ImageId): Promise<void> {
  await http.post(`${API_BASE_URL}/images/${imageId}/ai_tag/`, null, {
    headers: authHeaders(token),
  })
}
