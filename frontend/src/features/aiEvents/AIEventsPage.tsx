import { useEffect, useState } from 'react';
import { Alert, Button, Card, List, Progress, Space, Spin, Statistic, Tag, message } from 'antd';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useNavigate } from 'react-router-dom';
import {
  claimAIEvent,
  getAIEventHistory,
  getAIPointBalance,
  getCurrentAIEvent,
  getMyAIEventClaim,
} from '../../api/aiEvents';

export default function AIEventsPage({ token }: { token: string }) {
  const qc = useQueryClient(),
    nav = useNavigate(),
    [clock, setClock] = useState(Date.now()),
    [serverOffset, setServerOffset] = useState(0);
  useEffect(() => {
    const id = setInterval(() => setClock(Date.now()), 1000);
    return () => clearInterval(id);
  }, []);
  const event = useQuery({
    queryKey: ['ai-event'],
    queryFn: () => getCurrentAIEvent(token),
    refetchInterval: 15000,
  });
  const balance = useQuery({ queryKey: ['ai-points'], queryFn: () => getAIPointBalance(token) });
  const history = useQuery({
    queryKey: ['ai-event-history'],
    queryFn: () => getAIEventHistory(token),
  });
  useEffect(() => {
    if (event.data?.server_time) {
      setServerOffset(new Date(event.data.server_time).getTime() - Date.now());
    }
  }, [event.data?.server_time]);
  useEffect(() => {
    const sync = () => {
      if (document.visibilityState === 'visible') event.refetch();
    };
    document.addEventListener('visibilitychange', sync);
    return () => document.removeEventListener('visibilitychange', sync);
  }, [event]);
  const claim = useQuery({
    queryKey: ['ai-event-claim', event.data?.id],
    queryFn: () => getMyAIEventClaim(token, event.data!.id),
    enabled: !!event.data?.claimed,
    retry: false,
    refetchInterval: (q) =>
      ['queued', 'running'].includes(q.state.data?.status || '') ? 2000 : false,
  });
  const mutation = useMutation({
    mutationFn: () => claimAIEvent(token, event.data!.id),
    onSuccess: () => {
      message.success('领取成功，月报正在生成');
      qc.invalidateQueries({ queryKey: ['ai-event'] });
      qc.invalidateQueries({ queryKey: ['ai-event-claim'] });
    },
    onError: (e: any) => message.error(e?.response?.data?.message || '领取失败'),
  });
  if (event.isLoading || balance.isLoading) return <Spin />;
  if (!event.data || !balance.data) return <Alert type="warning" message="暂无活动" />;
  const now = clock + serverOffset;
  const opens = new Date(event.data.opens_at).getTime(),
    closes = new Date(event.data.closes_at).getTime(),
    seconds = Math.max(0, Math.ceil(((now < opens ? opens : closes) - now) / 1000)),
    open = now >= opens && now < closes;
  const phase = now < opens ? 'scheduled' : open ? 'open' : 'closed';
  const opensLabel = new Intl.DateTimeFormat('zh-CN', {
    timeZone: event.data.timezone,
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  }).format(new Date(event.data.opens_at));
  return (
    <Space direction="vertical" size="large" style={{ width: '100%' }}>
      <Card
        title="每日限量 AI 深度月报"
        extra={
          <Tag color={open ? 'green' : phase === 'closed' ? 'default' : 'blue'}>
            {open ? '领取中' : phase === 'closed' ? '已结束' : '未开放'}
          </Tag>
        }
      >
        <Space size="large" wrap>
          <Statistic
            title={now < opens ? '距离开放' : '距离结束'}
            value={`${Math.floor(seconds / 60)}:${String(seconds % 60).padStart(2, '0')}`}
          />
          <Statistic
            title="剩余名额"
            value={event.data.remaining_slots}
            suffix={`/ ${event.data.total_slots}`}
          />
          <Statistic title="固定点数" value={event.data.points_cost} />
          <Statistic title="可用点数" value={balance.data.available} />
        </Space>
        <Progress
          style={{ marginTop: 24 }}
          percent={Math.min(100, (event.data.streak_days / event.data.required_streak_days) * 100)}
          format={() => `连续 ${event.data.streak_days}/${event.data.required_streak_days} 天`}
        />
        <Button
          type="primary"
          size="large"
          disabled={
            !open ||
            !event.data.eligible ||
            event.data.claimed ||
            event.data.remaining_slots <= 0 ||
            balance.data.available < event.data.points_cost
          }
          loading={mutation.isPending}
          onClick={() => mutation.mutate()}
        >
          {event.data.claimed ? '今日已领取' : '立即领取'}
        </Button>
        {!event.data.eligible && (
          <Alert
            style={{ marginTop: 16 }}
            type="info"
            message={`连续记录天数不足（活动当天需在 ${opensLabel} 前完成）`}
          />
        )}
        {event.data.eligible && balance.data.available < event.data.points_cost && (
          <Alert style={{ marginTop: 16 }} type="warning" message="AI 点数不足" />
        )}
        {!event.data.claimed && event.data.remaining_slots <= 0 && (
          <Alert style={{ marginTop: 16 }} type="warning" message="本场名额已领完" />
        )}
        {event.data.claimed && (
          <Alert style={{ marginTop: 16 }} type="success" message="本场活动已经领取" />
        )}
      </Card>
      {claim.data && (
        <Card title="生成状态">
          <Tag>{claim.data.status}</Tag>
          {claim.data.status === 'succeeded' && claim.data.report_note_id && (
            <Button onClick={() => nav(`/notes/${claim.data!.report_note_id}`)}>
              查看自动写入的月报
            </Button>
          )}
          {claim.data.status === 'failed' && (
            <Alert type="error" message="生成失败，点数已返还；普通名额不返还" />
          )}
        </Card>
      )}
      <Card title="近期成功记录（完全匿名）">
        <List
          loading={history.isLoading}
          locale={{ emptyText: '暂无成功记录' }}
          dataSource={history.data || []}
          renderItem={(item) => (
            <List.Item>
              <List.Item.Meta
                title={item.display_name}
                description={new Date(item.claimed_at).toLocaleString()}
              />
            </List.Item>
          )}
        />
      </Card>
    </Space>
  );
}
