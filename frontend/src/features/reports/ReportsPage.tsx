import { useState } from 'react';
import {
  Alert,
  Button,
  Card,
  DatePicker,
  Input,
  List,
  Modal,
  Select,
  Space,
  Switch,
  TimePicker,
  message,
} from 'antd';
import dayjs from 'dayjs';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { confirmReport, previewReport, Source, streamPost } from '../../api/m2';
import {
  createScheduledReport,
  listScheduledReportRuns,
  ScheduledReportRun,
  listScheduledReports,
  retryScheduledReport,
  setScheduledReportEnabled,
} from '../../api/scheduledReports';
export default function ReportsPage() {
  const queryClient = useQueryClient();
  const [type, setType] = useState('weekly');
  const [anchor, setAnchor] = useState(dayjs());
  const [sources, setSources] = useState<Source[]>([]);
  const [content, setContent] = useState('');
  const [loading, setLoading] = useState(false);
  const [title, setTitle] = useState('周期报告');
  const [scheduleType, setScheduleType] = useState<'daily' | 'weekly' | 'monthly'>('weekly');
  const [scheduleTime, setScheduleTime] = useState(dayjs().hour(20).minute(0));
  const [runs, setRuns] = useState<ScheduledReportRun[]>([]);
  const [runsOpen, setRunsOpen] = useState(false);
  const tasks = useQuery({
    queryKey: ['scheduled-reports'],
    queryFn: () => listScheduledReports(),
  });
  const createSchedule = useMutation({
    mutationFn: () =>
      createScheduledReport({
        report_type: scheduleType,
        hour: scheduleTime.hour(),
        minute: scheduleTime.minute(),
        timezone: Intl.DateTimeFormat().resolvedOptions().timeZone || 'Asia/Shanghai',
      }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['scheduled-reports'] }),
  });
  async function load() {
    try {
      const p = await previewReport({
        type,
        anchor_date: anchor.format('YYYY-MM-DD'),
      });
      setSources(p.sources);
      if (!p.sources.length) message.warning('所选周期没有来源，不能生成报告');
    } catch (e) {
      message.error(e instanceof Error ? e.message : '加载失败');
    }
  }
  async function generate() {
    setLoading(true);
    setContent('');
    let out = '';
    try {
      await streamPost(
        '/reports/generate',
        { type, anchor_date: anchor.format('YYYY-MM-DD') },
        (c) => {
          out += c;
          setContent(out);
        },
      );
    } catch (e) {
      message.error(e instanceof Error ? e.message : '生成失败');
    } finally {
      setLoading(false);
    }
  }
  async function save() {
    try {
      await confirmReport({
        type,
        anchor_date: anchor.format('YYYY-MM-DD'),
        title,
        content,
        source_ids: sources.map((s) => s.id),
        overwrite: false,
      });
      message.success('报告已保存');
    } catch (e) {
      message.error(e instanceof Error ? e.message : '保存失败');
    }
  }
  return (
    <div className="feature-page">
      <h1>周期报告</h1>
      <Card>
        <Space wrap>
          <Select
            value={type}
            onChange={setType}
            options={[
              { value: 'daily', label: '日报' },
              { value: 'weekly', label: '周报' },
              { value: 'monthly', label: '月报' },
            ]}
          />
          <DatePicker value={anchor} onChange={(v) => v && setAnchor(v)} />
          <Button onClick={load}>选择来源</Button>
          <Button type="primary" disabled={!sources.length} loading={loading} onClick={generate}>
            生成草稿
          </Button>
        </Space>
      </Card>
      {!sources.length && (
        <Alert
          style={{ marginTop: 16 }}
          message={
            <span style={{ color: '#fff' }}>请先选择周期并加载来源；无来源时系统不会调用 AI。</span>
          }
        />
      )}
      <List
        header={`来源笔记（${sources.length}）`}
        dataSource={sources}
        renderItem={(s) => (
          <List.Item>
            <a href={`/notes/${s.id}`}>{s.title}</a>
            <span>{s.note_date}</span>
          </List.Item>
        )}
      />
      {content && (
        <Card title="报告预览">
          <Input value={title} onChange={(e) => setTitle(e.target.value)} />
          <Input.TextArea
            rows={16}
            value={content}
            onChange={(e) => setContent(e.target.value)}
            style={{ marginTop: 12 }}
          />
          <Button style={{ marginTop: 12 }} onClick={save}>
            确认保存
          </Button>
        </Card>
      )}
      <Card title="定时报告" style={{ marginTop: 16 }}>
        <Space wrap>
          <Select
            value={scheduleType}
            onChange={setScheduleType}
            options={[
              { value: 'daily', label: '每日' },
              { value: 'weekly', label: '每周日' },
              { value: 'monthly', label: '每月末' },
            ]}
          />
          <TimePicker
            value={scheduleTime}
            format="HH:mm"
            onChange={(v) => v && setScheduleTime(v)}
          />
          <Button
            type="primary"
            loading={createSchedule.isPending}
            onClick={() => createSchedule.mutate()}
          >
            新建定时任务
          </Button>
        </Space>
        <List
          loading={tasks.isLoading}
          dataSource={tasks.data || []}
          renderItem={(task) => (
            <List.Item
              actions={[
                <Switch
                  key="enabled"
                  checked={task.status === 'enabled'}
                  checkedChildren="启用"
                  unCheckedChildren="禁用"
                  onChange={async (enabled) => {
                    await setScheduledReportEnabled(task.id, enabled);
                    await tasks.refetch();
                  }}
                />,
                <Button
                  key="retry"
                  onClick={async () => {
                    await retryScheduledReport(task.id);
                    message.success('任务已加入执行队列');
                  }}
                >
                  立即执行/重试
                </Button>,
                <Button
                  key="runs"
                  onClick={async () => {
                    setRuns(await listScheduledReportRuns(task.id));
                    setRunsOpen(true);
                  }}
                >
                  执行记录
                </Button>,
              ]}
            >
              <List.Item.Meta
                title={`${task.report_type} · ${String(task.hour).padStart(2, '0')}:${String(task.minute).padStart(2, '0')}`}
                description={`下次执行：${new Date(task.next_run_at).toLocaleString()}${task.last_run_at ? `；上次执行：${new Date(task.last_run_at).toLocaleString()}` : ''}`}
              />
            </List.Item>
          )}
        />
      </Card>
      <Modal title="任务执行记录" open={runsOpen} footer={null} onCancel={() => setRunsOpen(false)}>
        <List
          locale={{ emptyText: '暂无执行记录' }}
          dataSource={runs}
          renderItem={(run) => (
            <List.Item>
              <List.Item.Meta
                title={`${run.status} · ${run.trigger} · ${new Date(run.started_at).toLocaleString()}`}
                description={
                  run.status === 'failed' ? (
                    `${run.error_code || 'ERROR'}：${run.error_message || '执行失败'}`
                  ) : run.report_note_id ? (
                    <a href={`/notes/${run.report_note_id}`}>查看生成的报告</a>
                  ) : (
                    '执行中'
                  )
                }
              />
            </List.Item>
          )}
        />
      </Modal>
    </div>
  );
}
