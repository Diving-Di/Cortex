import { http, authHeaders } from './http';

export type Preferences = {
  marketplace_personalization: boolean;
  version: number;
};

export async function getPreferences(token: string) {
  return (
    await http.get<Preferences>('/api/v1/settings/preferences', { headers: authHeaders(token) })
  ).data;
}

export async function updatePreferences(token: string, value: Preferences) {
  return (
    await http.put<Preferences>('/api/v1/settings/preferences', value, {
      headers: authHeaders(token),
    })
  ).data;
}
