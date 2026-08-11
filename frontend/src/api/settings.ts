import { http } from './http';

export type Preferences = {
  marketplace_personalization: boolean;
  version: number;
};

export async function getPreferences() {
  return (await http.get<Preferences>('/api/v1/settings/preferences', {})).data;
}

export async function updatePreferences(value: Preferences) {
  return (await http.put<Preferences>('/api/v1/settings/preferences', value, {})).data;
}
