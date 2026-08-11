import { http } from './http';

export type Dashboard = {
  date: string;
  timezone: string;
  today: { new_notes: number };
  streak_days: number;
  statistics: { notes: number; words: number; ai_requests: number; ai_tokens: number };
  recent_notes: Array<{
    id: number;
    title: string;
    type: string;
    note_date: string | null;
    updated_at: string;
    summary: string | null;
  }>;
  activity: Array<{ date: string; count: number }>;
  pending_reports: Array<{
    type: string;
    label: string;
    anchor_date: string;
    period_start: string;
  }>;
};

export async function getDashboard() {
  const timezone = Intl.DateTimeFormat().resolvedOptions().timeZone || 'Asia/Shanghai';
  return (
    await http.get<Dashboard>('/api/v1/dashboard', {
      params: { timezone },
    })
  ).data;
}
