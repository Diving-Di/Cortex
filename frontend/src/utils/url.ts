import { API_BASE_URL } from '../api/config'

export function getFullImageUrl(url?: string): string {
  if (!url) return ''
  if (url.startsWith('http') || url.startsWith('https')) {
    return url
  }
  const baseUrl = API_BASE_URL.replace(/\/api\/?$/, '')
  return `${baseUrl}${url}`
}
