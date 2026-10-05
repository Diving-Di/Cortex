import { useRef, useState } from 'react';
import { Alert, Button, Card, Input, List, Modal, Space, Tag, message } from 'antd';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import {
  cancelReportJob,
  confirmReportJob,
  createReportJob,
  getReportJob,
  listReportJobs,
  ReportJob,
} from '../../api/reportJobs';

const statusLabels = {
  queued: '排队中',
  running: '生成中',
  success: '草稿已完成',
  failed: '生成失败',
  cancelled: '已取消',
};
const failureMessages: Record<string, string> = {
  REPORT_WORKER_INTERRUPTED: '执行中断，请手动重试',
  REPORT_GENERATION_INTERRUPTED: '生成超时或中断，请手动重试',
  REPORT_SOURCES_CHANGED: '来源已修改或删除，请重新生成',
  REPORT_INVALID_CITATIONS: '生成内容缺少有效引用，请重试',
  AI_NOT_CONFIGURED: 'AI 尚未配置',
};

export default function BackgroundReports({
  type,
  anchorDate,
}: {
  type: string;
  anchorDate: string;
}) {
  const client = useQueryClient();
  const jobs = useQuery({
    queryKey: ['report-jobs'],
    queryFn: listReportJobs,
    refetchInterval: 3000,
  });
  const [submitting, setSubmitting] = useState(false);
  const [saving, setSaving] = useState(false);
  const [draft, setDraft] = useState<ReportJob>();
  const [title, setTitle] = useState('周期报告');
  const [content, setContent] = useState('');
  const [error, setError] = useState('');
  // Reuse the request ID when retrying a submission after a lost response.
  const pending = useRef<Record<string, string>>({});
  async function submit(kind = type, date = anchorDate) {
    if (submitting) return;
    const key = `${kind}:${date}`;
    pending.current[key] ||= crypto.randomUUID();
    setSubmitting(true);
    setError('');
    try {
      await createReportJob({ request_id: pending.current[key], type: kind, anchor_date: date });
      delete pending.current[key];
      await jobs.refetch();
      message.success('任务已提交，关闭页面后仍会继续生成');
    } catch (e) {
      setError(e instanceof Error ? e.message : '提交失败，请重试');
    } finally {
      setSubmitting(false);
    }
  }
  async function open(id: string) {
    try {
      const job = await getReportJob(id);
      setDraft(job);
      setTitle(`${job.anchor_date} 周期报告`);
      setContent(job.content);
    } catch (e) {
      message.error(e instanceof Error ? e.message : '草稿加载失败');
    }
  }
  async function save(overwrite = false) {
    if (!draft) return;
    setSaving(true);
    try {
      await confirmReportJob(draft.id, { title, content, overwrite });
      setDraft(undefined);
      message.success('报告已保存');
      await Promise.all(
        ['report-jobs', 'report-history', 'notes', 'dashboard'].map((key) =>
          client.invalidateQueries({ queryKey: [key] }),
        ),
      );
    } catch (e: any) {
      if (e?.response?.data?.code === 'REPORT_EXISTS' && !overwrite) {
        Modal.confirm({
          title: '该周期已有报告',
          content: '确认覆盖后会保存原正文为历史版本。',
          okText: '确认覆盖',
          onOk: () => save(true),
        });
      } else message.error(e instanceof Error ? e.message : '保存失败');
    } finally {
      setSaving(false);
    }
  }
  return (
    <Card title="后台报告" style={{ marginTop: 16 }}>
      <Space direction="vertical" style={{ width: '100%' }}>
        <p>按当前选择的周期生成报告。关闭页面后任务仍会继续，完成后在这里核对并确认保存。</p>
        <Button type="primary" loading={submitting} onClick={() => submit()}>
          后台生成草稿
        </Button>
        {error && <Alert type="warning" message={error} />}
        {jobs.isError && (
          <Alert
            type="warning"
            message="后台任务加载失败"
            action={<Button onClick={() => jobs.refetch()}>刷新任务</Button>}
          />
        )}
        <List
          loading={jobs.isLoading}
          dataSource={jobs.data || []}
          locale={{ emptyText: '暂无后台任务' }}
          renderItem={(job) => (
            <List.Item
              actions={[
                ...(job.status === 'success'
                  ? [
                      job.confirmed_note_id ? (
                        <a key="note" href={`/notes/${job.confirmed_note_id}`}>
                          查看报告
                        </a>
                      ) : (
                        <Button key="open" onClick={() => open(job.id)}>
                          核对草稿
                        </Button>
                      ),
                    ]
                  : []),
                ...(['failed', 'cancelled'].includes(job.status)
                  ? [
                      <Button
                        key="retry"
                        disabled={submitting}
                        onClick={() => submit(job.type, job.anchor_date)}
                      >
                        重新生成
                      </Button>,
                    ]
                  : []),
                ...(['queued', 'running'].includes(job.status)
                  ? [
                      <Button
                        key="cancel"
                        onClick={async () => {
                          try {
                            await cancelReportJob(job.id);
                            await jobs.refetch();
                          } catch {
                            message.error('取消失败，请刷新任务状态');
                          }
                        }}
                      >
                        取消任务
                      </Button>,
                    ]
                  : []),
              ]}
            >
              <List.Item.Meta
                title={
                  <span>
                    {job.anchor_date} ·{' '}
                    {
                      (
                        { daily: '日报', weekly: '周报', monthly: '月报' } as Record<string, string>
                      )[job.type]
                    }{' '}
                    <Tag>{statusLabels[job.status]}</Tag>
                  </span>
                }
                description={
                  job.failure_code
                    ? failureMessages[job.failure_code] || '生成失败，请手动重试'
                    : new Date(job.created_at).toLocaleString()
                }
              />
            </List.Item>
          )}
        />
      </Space>
      <Modal
        title="核对后台报告草稿"
        open={Boolean(draft)}
        width={800}
        onCancel={() => !saving && setDraft(undefined)}
        onOk={() => save()}
        okText="确认保存报告"
        confirmLoading={saving}
        okButtonProps={{ disabled: !title.trim() || !content.trim() }}
      >
        <Alert
          type="info"
          message="生成的原始草稿已保存在服务端；此窗口中的手动修改请确认保存后再关闭。"
        />
        <List
          dataSource={draft?.sources || []}
          renderItem={(source) => (
            <List.Item>
              <a href={`/notes/${source.id}`}>{source.title}</a>
            </List.Item>
          )}
        />
        <Input
          aria-label="后台报告标题"
          value={title}
          disabled={saving}
          onChange={(e) => setTitle(e.target.value)}
        />
        <Input.TextArea
          aria-label="后台报告正文"
          value={content}
          disabled={saving}
          rows={16}
          onChange={(e) => setContent(e.target.value)}
        />
      </Modal>
    </Card>
  );
}
