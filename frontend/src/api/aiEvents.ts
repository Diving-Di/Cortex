import { authHeaders, http } from './http';
export interface AIEvent {
  id: string;
  event_date: string;
  timezone: string;
  opens_at: string;
  closes_at: string;
  total_slots: number;
  remaining_slots: number;
  points_cost: number;
  required_streak_days: number;
  status: string;
  server_time: string;
  eligible: boolean;
  streak_days: number;
  claimed: boolean;
  show_dashboard_prompt: boolean;
}
export interface AIPointBalance {
  period_start: string;
  granted: number;
  consumed: number;
  held: number;
  available: number;
  version: number;
}
export interface AIEventClaim {
  id: number;
  event_id: string;
  status: 'queued' | 'running' | 'succeeded' | 'failed';
  points_cost: number;
  streak_days: number;
  claimed_at: string;
  report_note_id?: number;
  error_code?: string;
}
export interface AIEventHistoryItem {
  display_name: string;
  claimed_at: string;
}
const config = (token: string) => ({ headers: authHeaders(token) });
export async function getCurrentAIEvent(token: string) {
  return (await http.get<AIEvent>('/api/v1/ai-events/current', config(token))).data;
}
export async function getAIPointBalance(token: string) {
  return (await http.get<AIPointBalance>('/api/v1/ai-points/balance', config(token))).data;
}
export async function claimAIEvent(token: string, id: string) {
  return (
    await http.post<AIEventClaim>(
      `/api/v1/ai-events/${id}/claims`,
      {},
      { headers: { ...authHeaders(token), 'Idempotency-Key': crypto.randomUUID() } },
    )
  ).data;
}
export async function getMyAIEventClaim(token: string, id: string) {
  return (await http.get<AIEventClaim>(`/api/v1/ai-events/${id}/claims/me`, config(token))).data;
}
export async function getAIEventHistory(token: string) {
  return (
    await http.get<{ items: AIEventHistoryItem[] }>('/api/v1/ai-events/history', config(token))
  ).data.items;
}
