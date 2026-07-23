import { authHeaders, http } from './http';

export type ScheduledReportTask = {
  id: number;
  report_type: 'daily' | 'weekly' | 'monthly';
  hour: number;
  minute: number;
  timezone: string;
  status: 'enabled' | 'disabled';
  next_run_at: string;
  last_run_at: string | null;
};
export type ScheduledReportRun = {
  id: number;
  status: 'running' | 'success' | 'failed';
  trigger: 'scheduled' | 'manual';
  report_note_id: number | null;
  error_code: string | null;
  error_message: string | null;
  started_at: string;
  finished_at: string | null;
};

export async function listScheduledReports(token: string) {
  return (
    await http.get<ScheduledReportTask[]>('/api/v1/scheduled-reports', {
      headers: authHeaders(token),
    })
  ).data;
}
export async function createScheduledReport(token: string, body: object) {
  return (
    await http.post<ScheduledReportTask>('/api/v1/scheduled-reports', body, {
      headers: authHeaders(token),
    })
  ).data;
}
export async function setScheduledReportEnabled(token: string, id: number, enabled: boolean) {
  return (
    await http.patch<ScheduledReportTask>(
      `/api/v1/scheduled-reports/${id}`,
      {},
      { params: { enabled }, headers: authHeaders(token) },
    )
  ).data;
}
export async function retryScheduledReport(token: string, id: number) {
  await http.post(`/api/v1/scheduled-reports/${id}/retry`, {}, { headers: authHeaders(token) });
}
export async function listScheduledReportRuns(token: string, id: number) {
  return (
    await http.get<ScheduledReportRun[]>(`/api/v1/scheduled-reports/${id}/runs`, {
      headers: authHeaders(token),
    })
  ).data;
}
