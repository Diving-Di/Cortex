import { useEffect, useMemo, useState } from 'react';
import {
  Alert,
  Button,
  Card,
  Col,
  Empty,
  Input,
  List,
  Row,
  Space,
  Statistic,
  Tag,
  message,
} from 'antd';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { Link, useNavigate } from 'react-router-dom';
import { getDashboard } from '../../api/dashboard';
import { confirmOrganize, streamPost } from '../../api/m2';
import { getCurrentAIEvent } from '../../api/aiEvents';
import SafeMarkdown from '../../components/SafeMarkdown';
import './Dashboard.css';
import PendingWork from './PendingWork';
import ContentDiff from '../../components/ContentDiff';
import { useDraftScope, readDraft, writeDraft, removeDraft } from '../../app/drafts';
import { createNote } from '../../api/notes';

function OfflineStatus() {
  const [online, setOnline] = useState(navigator.onLine);
  useEffect(() => {
    const update = () => setOnline(navigator.onLine);
    window.addEventListener('online', update);
    window.addEventListener('offline', update);
    return () => {
      window.removeEventListener('online', update);
      window.removeEventListener('offline', update);
    };
  }, []);
  return online ? null : (
    <Alert
      showIcon
      type="warning"
      message="当前处于离线状态"
      description="应用外壳仍可使用；数据功能会在恢复连接后重新加载。"
    />
  );
}

export default function DashboardPage() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const scope = useDraftScope();
  const initialDraft = useMemo(
    () =>
      readDraft<{
        raw: string;
        preview: { title: string; content: string; summary?: string } | null;
        sourceRaw: string;
      }>(scope, 'organize'),
    [scope],
  );
  const [sourceRaw, setSourceRaw] = useState(initialDraft?.value.sourceRaw || '');
  const [saving, setSaving] = useState(false);
  const [storageFailed, setStorageFailed] = useState(false);
  const dashboard = useQuery({
    queryKey: ['dashboard'],
    queryFn: () => getDashboard(),
    retry: navigator.onLine ? 1 : false,
  });
  const aiEvent = useQuery({ queryKey: ['ai-event'], queryFn: () => getCurrentAIEvent() });
  const [raw, setRaw] = useState(initialDraft?.value.raw || '');
  const [draft, setDraft] = useState('');
  const [loading, setLoading] = useState(false);
  const [preview, setPreview] = useState<{
    title: string;
    summary?: string;
    content: string;
  } | null>(initialDraft?.value.preview || null);
  useEffect(() => {
    if (!raw && !preview) return;
    setStorageFailed(
      !writeDraft(scope, {
        key: 'organize',
        label: '快速记录 / AI 整理草稿',
        path: '/',
        value: { raw, preview, sourceRaw },
      }),
    );
  }, [raw, preview, sourceRaw, scope]);

  const activity = useMemo(() => {
    const counts = new Map(dashboard.data?.activity.map((item) => [item.date, item.count]));
    const days = [];
    const cursor = new Date();
    cursor.setHours(12, 0, 0, 0);
    for (let index = 83; index >= 0; index -= 1) {
      const day = new Date(cursor);
      day.setDate(cursor.getDate() - index);
      const key = `${day.getFullYear()}-${String(day.getMonth() + 1).padStart(2, '0')}-${String(day.getDate()).padStart(2, '0')}`;
      days.push({ date: key, count: counts.get(key) || 0 });
    }
    return days;
  }, [dashboard.data]);

  async function organize() {
    setLoading(true);
    setDraft('');
    let out = '';
    try {
      await streamPost('/ai/organize', { content: raw }, (content) => {
        out += content;
        setDraft(out);
      });
      const parsed = JSON.parse(out.replace(/^```json\s*|\s*```$/g, ''));
      if (typeof parsed.title !== 'string' || typeof parsed.content !== 'string')
        throw new Error('整理结果格式无效，原草稿已保留');
      setPreview(parsed);
      setSourceRaw(raw);
    } catch (error) {
      message.error(error instanceof Error ? error.message : '整理失败');
    } finally {
      setLoading(false);
    }
  }

  async function save() {
    if (!preview || raw !== sourceRaw || saving) return;
    setSaving(true);
    try {
      const note = await confirmOrganize(preview);
      message.success(`已保存笔记 #${note.id}`);
      setRaw('');
      setDraft('');
      setPreview(null);
      removeDraft(scope, 'organize');
      await queryClient.invalidateQueries({ queryKey: ['dashboard'] });
    } catch (error) {
      message.error(error instanceof Error ? error.message : '保存失败');
    } finally {
      setSaving(false);
    }
  }

  const data = dashboard.data;
  const eventTime = aiEvent.data
    ? new Intl.DateTimeFormat('zh-CN', {
        timeZone: aiEvent.data.timezone,
        hour: '2-digit',
        minute: '2-digit',
        hour12: false,
      }).format(new Date(aiEvent.data.opens_at))
    : '';
  const eventDuration = aiEvent.data
    ? Math.round(
        (new Date(aiEvent.data.closes_at).getTime() - new Date(aiEvent.data.opens_at).getTime()) /
          60000,
      )
    : 0;
  return (
    <div className="feature-page dashboard-page">
      <OfflineStatus />
      {storageFailed && <Alert type="warning" message="浏览器草稿保存失败，请及时保存或复制记录" />}
      <div className="dashboard-heading">
        <div>
          <h1>工作台</h1>
          <span>{data?.date || '正在加载今日摘要…'}</span>
        </div>
        <Button onClick={() => dashboard.refetch()}>刷新</Button>
      </div>
      {dashboard.isError && (
        <Alert type="error" showIcon message="摘要加载失败" description="请检查后端连接后重试。" />
      )}
      <Row gutter={[12, 12]} className="dashboard-stats">
        <Col xs={12} lg={6}>
          <Card>
            <Statistic title="今日新笔记" value={data?.today.new_notes || 0} />
          </Card>
        </Col>
        <Col xs={12} lg={6}>
          <Card>
            <Statistic title="连续记录" value={data?.streak_days || 0} suffix="天" />
          </Card>
        </Col>
        <Col xs={12} lg={6}>
          <Card>
            <Statistic title="笔记总数" value={data?.statistics.notes || 0} />
          </Card>
        </Col>
        <Col xs={12} lg={6}>
          <Card>
            <Statistic
              title="AI 估算用量"
              value={data?.statistics.ai_tokens || 0}
              suffix="tokens"
            />
          </Card>
        </Col>
      </Row>
      <Row gutter={[16, 16]}>
        <Col xs={24} lg={15}>
          <Card title="快速记录">
            <Input.TextArea
              disabled={loading || saving}
              value={raw}
              onChange={(event) => setRaw(event.target.value)}
              rows={6}
              placeholder="先把想法记下来，AI 只生成草稿，不会自动覆盖或保存。"
            />
            <Space style={{ marginTop: 16 }}>
              <Button type="primary" loading={loading} disabled={!raw.trim()} onClick={organize}>
                AI 整理
              </Button>
              <Button
                loading={saving}
                disabled={!raw.trim() || loading}
                onClick={async () => {
                  setSaving(true);
                  try {
                    await createNote({
                      type: 'normal',
                      title: raw.trim().split('\n')[0].slice(0, 80) || '快速记录',
                      content: raw,
                      note_date: new Date().toLocaleDateString('sv-SE'),
                    });
                    setRaw('');
                    setPreview(null);
                    removeDraft(scope, 'organize');
                    await queryClient.invalidateQueries({ queryKey: ['dashboard'] });
                    message.success('原文已保存');
                  } catch {
                    message.error('保存失败，原文仍保留');
                  } finally {
                    setSaving(false);
                  }
                }}
              >
                保存原文
              </Button>
              {preview && (
                <Button disabled={loading || raw !== sourceRaw} loading={saving} onClick={save}>
                  确认保存整理稿
                </Button>
              )}
            </Space>
          </Card>
          {draft && (
            <Card title="整理预览" style={{ marginTop: 16 }}>
              {preview ? (
                <>
                  <Input
                    value={preview.title}
                    onChange={(event) => setPreview({ ...preview, title: event.target.value })}
                  />
                  <Input.TextArea
                    style={{ marginTop: 12 }}
                    rows={10}
                    value={preview.content}
                    onChange={(event) => setPreview({ ...preview, content: event.target.value })}
                  />
                  <ContentDiff before={sourceRaw} after={preview.content} />
                  <SafeMarkdown>{preview.content}</SafeMarkdown>
                </>
              ) : (
                <>
                  <Alert type="info" message="正在生成结构化草稿…" />
                  <pre>{draft}</pre>
                </>
              )}
            </Card>
          )}
        </Col>
        <Col xs={24} lg={9}>
          <PendingWork />
          <Card title="最近笔记" extra={<Link to="/notes">查看全部</Link>}>
            <List
              dataSource={data?.recent_notes || []}
              locale={{
                emptyText: <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="还没有笔记" />,
              }}
              renderItem={(note) => (
                <List.Item className="recent-note" onClick={() => navigate(`/notes/${note.id}`)}>
                  <List.Item.Meta
                    title={note.title}
                    description={
                      note.summary || note.note_date || new Date(note.updated_at).toLocaleString()
                    }
                  />
                </List.Item>
              )}
            />
          </Card>
          <Card title="待生成报告" style={{ marginTop: 16 }}>
            {data?.pending_reports.length ? (
              <Space wrap>
                {data.pending_reports.map((item) => (
                  <Tag
                    key={item.type}
                    color="blue"
                    className="report-tag"
                    onClick={() => navigate('/reports')}
                  >
                    {item.label} · {item.period_start}
                  </Tag>
                ))}
              </Space>
            ) : (
              <span className="muted">当前没有待生成报告</span>
            )}
          </Card>
        </Col>
      </Row>
      <Card
        title="免费 AI 点数活动"
        style={{ marginTop: 16 }}
        extra={<Link to="/ai-events">查看活动</Link>}
      >
        {aiEvent.data ? (
          <span>
            {eventTime} 开放 · {eventDuration} 分钟 · {aiEvent.data.total_slots} 个名额 ·{' '}
            {aiEvent.data.points_reward} 点
          </span>
        ) : (
          <span>活动信息暂不可用，记录和整理入口仍可使用。</span>
        )}
      </Card>
      <Card
        title="近 12 周记录活跃度"
        style={{ marginTop: 16 }}
        extra={
          <span>
            {data?.statistics.words || 0} 字 · {data?.statistics.ai_requests || 0} 次 AI 请求
          </span>
        }
      >
        <div className="activity-grid" aria-label="近 12 周记录活跃度">
          {activity.map((item) => (
            <span
              key={item.date}
              title={`${item.date}: ${item.count} 篇`}
              className={`activity-cell level-${Math.min(item.count, 4)}`}
            />
          ))}
        </div>
      </Card>
    </div>
  );
}
