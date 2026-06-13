import type { ImageItem, TagRecord } from '../types/image'
import { normalizeTagNames } from './tags'

export function getCustomTags(item: ImageItem): TagRecord[] {
  const tags = item.tags || []
  const seen = new Set<string>()
  const result: TagRecord[] = []

  for (const tag of tags) {
    if (tag.tag_type && tag.tag_type !== 'Custom') continue
    const names = normalizeTagNames(tag.tag_name || '')
    for (const name of names) {
      if (seen.has(name)) continue
      seen.add(name)
      result.push({ ...tag, tag_name: name })
    }
  }

  return result
}

export function getAiTags(item: ImageItem): TagRecord[] {
  const tags = item.tags || []
  const seen = new Set<string>()
  const result: TagRecord[] = []

  for (const tag of tags) {
    if (tag.tag_type !== 'AI') continue
    const name = (tag.tag_name || '').toString().trim()
    if (!name || seen.has(name)) continue
    seen.add(name)
    result.push({ ...tag, tag_name: name })
  }

  return result
}
