export function normalizeTagNames(raw: unknown): string[] {
  if (raw === null || raw === undefined) return []
  const s = String(raw).trim()
  if (!s) return []

  if (s.startsWith('[') && s.endsWith(']')) {
    try {
      const parsed = JSON.parse(s)
      if (Array.isArray(parsed)) {
        return parsed
          .map((x: unknown) => (x === null || x === undefined) ? '' : String(x).trim())
          .filter(Boolean)
          .filter(x => x.replace(/\s+/g, '') !== '[]')
          .filter(x => x.replace(/\s+/g, '') !== '[""]')
          .filter(x => x.replace(/\s+/g, '') !== '["\\"]')
      }
    } catch (e) {
      console.error(e)
    }
  }

  if (s.replace(/\s+/g, '') === '[]') return []
  if (s.replace(/\s+/g, '') === '[""]') return []
  if (s.replace(/\s+/g, '') === '["\\"]') return []
  return [s]
}
