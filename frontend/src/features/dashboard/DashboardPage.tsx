import { useEffect, useMemo, useState } from 'react';
import {
  Alert,
  Button,
  Card,
  Col,
  Empty,
  Input,
  List,
  Modal,
  Row,
  Space,
  Statistic,
  Tag,
  message,
} from 'antd';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { Link, useNavigate } from 'react-router-dom';
import ReactMarkdown from 'react-markdown';
import { getDashboard } from '../../api/dashboard';
import { confirmOrganize, streamPost } from '../../api/m2';
import { getCurrentAIEvent } from '../../api/aiEvents';
import './Dashboard.css';

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
  const dashboard = useQuery({
    queryKey: ['dashboard'],
    queryFn: () => getDashboard(),
    retry: navigator.onLine ? 1 : false,
  });
  const aiEvent = useQuery({ queryKey: ['ai-event'], queryFn: () => getCurrentAIEvent() });
  const [eventOpen, setEventOpen] = useState(false);
  useEffect(() => {
    if (!aiEvent.data) return;
    const key = `ai-event-modal-dismissed:${aiEvent.data.id}`;
    if (aiEvent.data.show_dashboard_prompt && !localStorage.getItem(key)) setEventOpen(true);
  }, [aiEvent.data]);
  const [raw, setRaw] = useState('');
  const [draft, setDraft] = useState('');
  const [loading, setLoading] = useState(false);
  const [preview, setPreview] = useState<{
    title: string;
    summary?: string;
    content: string;
  } | null>(null);

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
    setPreview(null);
    let out = '';
    try {
      await streamPost('/ai/organize', { content: raw }, (content) => {
        out += content;
        setDraft(out);
      });
      setPreview(JSON.parse(out.replace(/^```json\s*|\s*```$/g, '')));
    } catch (error) {
      message.error(error instanceof Error ? error.message : '整理失败');
    } finally {
      setLoading(false);
    }
  }

  async function save() {
    if (!preview) return;
    try {
      const note = await confirmOrganize(preview);
      message.success(`已保存笔记 #${note.id}`);
      setRaw('');
      setDraft('');
      setPreview(null);
      await queryClient.invalidateQueries({ queryKey: ['dashboard'] });
    } catch (error) {
      message.error(error instanceof Error ? error.message : '保存失败');
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
      <Modal
        open={eventOpen}
        title={eventTime ? `今晚 ${eventTime} AI 深度月报限量开放` : 'AI 深度月报限量开放'}
        onOk={() => navigate('/ai-events')}
        okText="查看活动"
        cancelText="今日不再提醒"
        onCancel={() => {
          if (aiEvent.data)
            localStorage.setItem(`ai-event-modal-dismissed:${aiEvent.data.id}`, '1');
          setEventOpen(false);
        }}
      >
        {aiEvent.data && (
          <p>
            持续 {eventDuration} 分钟，共 {aiEvent.data.total_slots} 个名额，固定消耗{' '}
            {aiEvent.data.points_cost} 点。连续记录 {aiEvent.data.required_streak_days}{' '}
            天（含活动当天）即可参与。
          </p>
        )}
      </Modal>
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
            <Statistic title="AI 用量" value={data?.statistics.ai_tokens || 0} suffix="tokens" />
          </Card>
        </Col>
      </Row>
      <Row gutter={[16, 16]}>
        <Col xs={24} lg={15}>
          <Card title="快速记录">
            <Input.TextArea
              value={raw}
              onChange={(event) => setRaw(event.target.value)}
              rows={6}
              placeholder="先把想法记下来，AI 只生成草稿，不会自动覆盖或保存。"
            />
            <Space style={{ marginTop: 16 }}>
              <Button type="primary" loading={loading} disabled={!raw.trim()} onClick={organize}>
                AI 整理
              </Button>
              {preview && <Button onClick={save}>确认保存</Button>}
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
                  <ReactMarkdown>{preview.content}</ReactMarkdown>
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
