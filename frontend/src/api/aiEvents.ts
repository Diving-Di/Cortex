import { http } from './http';
export interface AIEvent {
  id: string;
  event_date: string;
  timezone: string;
  opens_at: string;
  closes_at: string;
  total_slots: number;
  remaining_slots: number;
  points_reward: number;
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
  status: 'succeeded';
  points_reward: number;
  streak_days: number;
  claimed_at: string;
}
export interface AIEventHistoryItem {
  display_name: string;
  claimed_at: string;
}
export async function getCurrentAIEvent() {
  return (await http.get<AIEvent>('/api/v1/ai-events/current')).data;
}
export async function getAIPointBalance() {
  return (await http.get<AIPointBalance>('/api/v1/ai-points/balance')).data;
}
export async function claimAIEvent(id: string) {
  return (
    await http.post<AIEventClaim>(
      `/api/v1/ai-events/${id}/claims`,
      {},
      { headers: { 'Idempotency-Key': crypto.randomUUID() } },
    )
  ).data;
}
export async function getMyAIEventClaim(id: string) {
  return (await http.get<AIEventClaim>(`/api/v1/ai-events/${id}/claims/me`)).data;
}
export async function getAIEventHistory() {
  return (await http.get<{ items: AIEventHistoryItem[] }>('/api/v1/ai-events/history')).data.items;
}
