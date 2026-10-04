import { useContext, useEffect, useRef, useState } from 'react';
import {
  Alert,
  Button,
  Card,
  Collapse,
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
import { UNSAFE_DataRouterContext, useSearchParams } from 'react-router-dom';
import { useDraftScope, readDraft, writeDraft, removeDraft } from '../../app/drafts';
import { listNotes } from '../../api/notes';
import ContentDiff from '../../components/ContentDiff';
import UsageGuide from '../../components/UsageGuide';
import LeaveGuard from '../../components/LeaveGuard';
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
type GeneratedVersion = { content: string; sources: Source[]; createdAt: string };
export default function ReportsPage() {
  const queryClient = useQueryClient();
  const dataRouter = useContext(UNSAFE_DataRouterContext);
  const scope = useDraftScope();
  const epoch = useRef(0);
  const controller = useRef<AbortController>();
  const [sourceLoading, setSourceLoading] = useState(false);
  const [complete, setComplete] = useState(false);
  const [partial, setPartial] = useState('');
  const [generationError, setGenerationError] = useState('');
  const [loadedKey, setLoadedKey] = useState('');
  const [versions, setVersions] = useState<GeneratedVersion[]>([]);
  const [selectedVersion, setSelectedVersion] = useState<GeneratedVersion>();
  const [saving, setSaving] = useState(false);
  const [storageFailed, setStorageFailed] = useState(false);
  const [params] = useSearchParams();
  const [type, setType] = useState(
    ['daily', 'weekly', 'monthly'].includes(params.get('type') || '')
      ? params.get('type')!
      : 'weekly',
  );
  const [anchor, setAnchor] = useState(
    params.get('date') && dayjs(params.get('date')).isValid() ? dayjs(params.get('date')) : dayjs(),
  );
  const [previewSources, setPreviewSources] = useState<Source[]>([]);
  const [previewLoaded, setPreviewLoaded] = useState(false);
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
    onError: () => message.error('定时任务创建失败'),
  });
  const start =
    type === 'monthly'
      ? anchor.startOf('month')
      : type === 'weekly'
        ? anchor.subtract((anchor.day() + 6) % 7, 'day')
        : anchor;
  const periodKey = `${type}:${start.format('YYYY-MM-DD')}`;
  const draftKey = `report:${periodKey}`;
  const history = useQuery({
    queryKey: ['report-history', periodKey],
    queryFn: () =>
      listNotes({
        type,
        start_date: start.format('YYYY-MM-DD'),
        end_date: start.format('YYYY-MM-DD'),
      }),
  });
  useEffect(() => {
    epoch.current++;
    controller.current?.abort();
    const draft = readDraft<{
      title: string;
      content: string;
      sources: Source[];
      complete: boolean;
      versions?: GeneratedVersion[];
    }>(scope, draftKey);
    setVersions(draft?.value.versions || []);
    setSelectedVersion(undefined);
    setTitle(draft?.value.title || '周期报告');
    setContent(draft?.value.content || '');
    setSources(draft?.value.sources || []);
    setComplete(draft?.value.complete || false);
    setPartial('');
    setPreviewSources([]);
    setPreviewLoaded(false);
    setGenerationError('');
    setLoadedKey(draftKey);
    setLoading(false);
    setSourceLoading(false);
  }, [draftKey, scope]);
  useEffect(
    () => () => {
      epoch.current++;
      controller.current?.abort();
    },
    [],
  );
  useEffect(() => {
    if (loadedKey !== draftKey || !content) return;
    setStorageFailed(
      !writeDraft(scope, {
        key: draftKey,
        label: `${type} ${start.format('YYYY-MM-DD')} 草稿`,
        path: `/reports?type=${type}&date=${start.format('YYYY-MM-DD')}`,
        value: { title, content, sources, complete, versions },
      }),
    );
  }, [content, title, sources, complete, versions, draftKey, loadedKey, scope]);
  async function load() {
    const ticket = ++epoch.current;
    setSourceLoading(true);
    try {
      const p = await previewReport({ type, anchor_date: anchor.format('YYYY-MM-DD') });
      if (ticket !== epoch.current) return;
      setPreviewSources(p.sources);
      setPreviewLoaded(true);
      if (!p.sources.length) message.warning('所选周期没有来源，不能生成报告');
    } catch (e) {
      if (ticket === epoch.current) message.error(e instanceof Error ? e.message : '加载失败');
    } finally {
      if (ticket === epoch.current) setSourceLoading(false);
    }
  }
  async function generate() {
    const ticket = ++epoch.current;
    const abort = new AbortController();
    controller.current = abort;
    setLoading(true);
    setPartial('');
    setGenerationError('');
    let out = '';
    let generatedSources: Source[] = [];
    try {
      await streamPost(
        '/reports/generate',
        { type, anchor_date: anchor.format('YYYY-MM-DD') },
        (chunk) => {
          if (ticket !== epoch.current) return;
          out += chunk;
          setPartial(out);
        },
        {
          signal: abort.signal,
          onSources: (items) => {
            generatedSources = items;
          },
        },
      );
      if (ticket !== epoch.current) return;
      if (!generatedSources.length) throw new Error('生成结果未提供来源，请重新生成');
      setVersions((old) =>
        [
          { content: out, sources: generatedSources, createdAt: new Date().toISOString() },
          ...old,
        ].slice(0, 5),
      );
      setContent(out);
      setSources(generatedSources);
      setComplete(true);
      setPartial('');
    } catch (e) {
      if (ticket === epoch.current)
        setGenerationError(
          abort.signal.aborted
            ? '生成已停止，原草稿仍保留'
            : e instanceof Error
              ? e.message
              : '生成失败',
        );
    } finally {
      if (ticket === epoch.current) setLoading(false);
    }
  }
  async function save(overwrite = false) {
    if (!complete || loading || saving) return;
    setSaving(true);
    try {
      await confirmReport({
        type,
        anchor_date: start.format('YYYY-MM-DD'),
        title,
        content,
        source_ids: sources.map((s) => s.id),
        overwrite,
      });
      removeDraft(scope, draftKey);
      setContent('');
      setVersions([]);
      setSelectedVersion(undefined);
      setComplete(false);
      message.success('报告已保存');
      await queryClient.invalidateQueries({ queryKey: ['report-history'] });
      await queryClient.invalidateQueries({ queryKey: ['notes'] });
      await queryClient.invalidateQueries({ queryKey: ['dashboard'] });
    } catch (e: any) {
      if (e?.response?.data?.code === 'REPORT_EXISTS' && !overwrite) {
        Modal.confirm({
          title: '该周期已有报告',
          content: '覆盖会保留原正文为历史版本，请先核对来源和差异。',
          okText: '确认覆盖',
          onOk: () => save(true),
        });
      } else message.error(e instanceof Error ? e.message : '保存失败');
    } finally {
      setSaving(false);
    }
  }
  return (
    <div className="feature-page">
      <h1>周期报告</h1>
      {dataRouter && <LeaveGuard dirty={Boolean(content)} />}
      <UsageGuide
        id="reports"
        steps={[
          { title: '选择周期', description: '加载所选周期来源，没有资料时不会调用 AI。' },
          {
            title: '生成与核对',
            description: '草稿和来源绑定，重新生成失败会保留上一份完整草稿。',
          },
          { title: '确认保存', description: '核对差异后保存，历史报告可在笔记中查看和恢复。' },
        ]}
      />
      {storageFailed && <Alert type="warning" message="浏览器无法保存草稿，请复制内容后再离开" />}
      <Card>
        <Space wrap>
          <Select
            disabled={loading || saving}
            value={type}
            onChange={setType}
            options={[
              { value: 'daily', label: '日报' },
              { value: 'weekly', label: '周报' },
              { value: 'monthly', label: '月报' },
            ]}
          />
          <DatePicker
            disabled={loading || saving}
            value={anchor}
            onChange={(v) => v && setAnchor(v)}
          />
          <Button loading={sourceLoading} disabled={loading || saving} onClick={load}>
            选择来源
          </Button>
          <Button
            type="primary"
            disabled={
              !(previewLoaded ? previewSources.length : sources.length) || sourceLoading || saving
            }
            loading={loading}
            onClick={generate}
          >
            生成草稿
          </Button>
          {loading && <Button onClick={() => controller.current?.abort()}>停止生成</Button>}
        </Space>
      </Card>
      {!(previewLoaded ? previewSources.length : sources.length) && (
        <Alert
          style={{ marginTop: 16 }}
          message={<span>请先选择周期并加载来源；无来源时系统不会调用 AI。</span>}
        />
      )}
      <List
        header={`${content ? '当前草稿来源' : '待生成来源'}（${(content ? sources : previewSources).length}）`}
        dataSource={content ? sources : previewSources}
        renderItem={(s) => (
          <List.Item>
            <a href={`/notes/${s.id}`}>{s.title}</a>
            <span>{s.note_date}</span>
          </List.Item>
        )}
      />
      {previewLoaded && content && (
        <p>已加载当前周期 {previewSources.length} 条来源，重新生成成功后才替换草稿及其来源。</p>
      )}
      {generationError && <Alert type="warning" message={generationError} />}
      {partial && (
        <Collapse
          items={[
            {
              key: 'partial',
              label: loading ? '正在生成' : '未完成尝试（不可保存为报告）',
              children: <pre style={{ whiteSpace: 'pre-wrap' }}>{partial}</pre>,
            },
          ]}
        />
      )}
      {versions.length > 0 && (
        <Collapse
          items={[
            {
              key: 'versions',
              label: `当前标签页生成历史（${versions.length}）`,
              children: (
                <>
                  <List
                    dataSource={versions}
                    renderItem={(version, index) => (
                      <List.Item>
                        <Button onClick={() => setSelectedVersion(version)}>
                          第 {versions.length - index} 次成功草稿 ·{' '}
                          {new Date(version.createdAt).toLocaleString()}
                        </Button>
                      </List.Item>
                    )}
                  />
                  {selectedVersion && (
                    <>
                      <ContentDiff before={content} after={selectedVersion.content} />
                      <Button
                        disabled={loading || saving}
                        onClick={() => {
                          setContent(selectedVersion.content);
                          setSources(selectedVersion.sources);
                          setComplete(true);
                        }}
                      >
                        恢复为待确认草稿
                      </Button>
                    </>
                  )}
                </>
              ),
            },
          ]}
        />
      )}
      <Card title="已保存报告 / 历史">
        {history.isError && (
          <Alert
            type="warning"
            message="报告历史加载失败"
            action={<Button onClick={() => history.refetch()}>重试</Button>}
          />
        )}
        <List
          dataSource={history.data?.items || []}
          renderItem={(n) => (
            <List.Item>
              <a href={`/notes/${n.id}`}>
                {n.title} · {new Date(n.updated_at).toLocaleString()} · 查看版本
              </a>
            </List.Item>
          )}
        />
      </Card>
      {content && (
        <Card title="报告预览">
          <Input
            disabled={saving || loading}
            value={title}
            onChange={(e) => setTitle(e.target.value)}
          />
          <Input.TextArea
            disabled={saving || loading}
            rows={16}
            value={content}
            onChange={(e) => setContent(e.target.value)}
            style={{ marginTop: 12 }}
          />
          <Collapse
            items={[
              {
                key: 'diff',
                label: '核对与已保存报告的差异',
                children: (
                  <ContentDiff before={history.data?.items[0]?.content || ''} after={content} />
                ),
              },
            ]}
          />
          <Button
            disabled={!complete || loading}
            loading={saving}
            style={{ marginTop: 12 }}
            onClick={() => save()}
          >
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
                    try {
                      await setScheduledReportEnabled(task.id, enabled);
                      await tasks.refetch();
                    } catch {
                      message.error('任务状态修改失败');
                    }
                  }}
                />,
                <Button
                  key="retry"
                  onClick={async () => {
                    try {
                      await retryScheduledReport(task.id);
                      message.success('任务已加入执行队列');
                    } catch {
                      message.error('任务重试失败');
                    }
                  }}
                >
                  立即执行/重试
                </Button>,
                <Button
                  key="runs"
                  onClick={async () => {
                    try {
                      setRuns(await listScheduledReportRuns(task.id));
                      setRunsOpen(true);
                    } catch {
                      message.error('执行记录加载失败');
                    }
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
