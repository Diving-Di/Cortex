import { http } from './http';
import type { Source } from './m2';

export type ReportJob = {
  id: string;
  request_id: string;
  type: string;
  anchor_date: string;
  status: 'queued' | 'running' | 'success' | 'failed' | 'cancelled';
  stage: string;
  sources: Source[];
  content: string;
  failure_code: string | null;
  confirmed_note_id: number | null;
  created_at: string;
  updated_at: string;
};
export async function listReportJobs() {
  return (await http.get<ReportJob[]>('/api/v1/report-jobs')).data;
}
export async function createReportJob(body: {
  request_id: string;
  type: string;
  anchor_date: string;
}) {
  return (await http.post<ReportJob>('/api/v1/report-jobs', body)).data;
}
export async function getReportJob(id: string) {
  return (await http.get<ReportJob>(`/api/v1/report-jobs/${id}`)).data;
}
export async function cancelReportJob(id: string) {
  return (await http.post(`/api/v1/report-jobs/${id}/cancel`)).data;
}
export async function confirmReportJob(
  id: string,
  body: { title: string; content: string; overwrite: boolean },
) {
  return (await http.post<{ id: number }>(`/api/v1/report-jobs/${id}/confirm`, body)).data;
}
