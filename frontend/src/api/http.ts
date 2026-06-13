import axios from 'axios'

export const http = axios

export function authHeaders(token: string) {
  return { Authorization: `Token ${token}` }
}
