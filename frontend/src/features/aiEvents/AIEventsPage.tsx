import { useEffect, useState } from 'react';
import { Alert, Button, Card, List, Progress, Space, Spin, Statistic, Tag, message } from 'antd';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { claimAIEvent, getAIEventPage } from '../../api/aiEvents';

export default function AIEventsPage() {
  const qc = useQueryClient(),
    [clock, setClock] = useState(Date.now()),
    [serverOffset, setServerOffset] = useState(0);
  useEffect(() => {
    const id = setInterval(() => setClock(Date.now()), 1000);
    return () => clearInterval(id);
  }, []);
  const page = useQuery({
    queryKey: ['ai-event-page'],
    queryFn: () => getAIEventPage(),
    refetchInterval: 15000,
  });
  const event = page.data?.event;
  const balance = page.data?.balance;
  const history = page.data?.history;
  useEffect(() => {
    if (event?.server_time) {
      setServerOffset(new Date(event.server_time).getTime() - Date.now());
    }
  }, [event?.server_time]);
  useEffect(() => {
    const sync = () => {
      if (document.visibilityState === 'visible') page.refetch();
    };
    document.addEventListener('visibilitychange', sync);
    return () => document.removeEventListener('visibilitychange', sync);
  }, [page]);
  const mutation = useMutation({
    mutationFn: () => claimAIEvent(event!.id),
    onSuccess: () => {
      message.success(`领取成功，已到账 ${event!.points_reward} 点`);
      qc.invalidateQueries({ queryKey: ['ai-event-page'] });
    },
    onError: (e: any) => message.error(e?.response?.data?.message || '领取失败'),
  });
  if (page.isLoading) return <Spin />;
  if (!event || !balance) return <Alert type="warning" message="暂无活动" />;
  const now = clock + serverOffset;
  const opens = new Date(event.opens_at).getTime(),
    closes = new Date(event.closes_at).getTime(),
    seconds = Math.max(0, Math.ceil(((now < opens ? opens : closes) - now) / 1000)),
    open = now >= opens && now < closes;
  const phase = now < opens ? 'scheduled' : open ? 'open' : 'closed';
  const opensLabel = new Intl.DateTimeFormat('zh-CN', {
    timeZone: event.timezone,
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  }).format(new Date(event.opens_at));
  return (
    <Space direction="vertical" size="large" style={{ width: '100%' }}>
      <Card
        title="每日限量免费点数"
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
            value={event.remaining_slots}
            suffix={`/ ${event.total_slots}`}
          />
          <Statistic title="本次赠送" value={event.points_reward} suffix="点" />
          <Statistic title="可用点数" value={balance.available} />
        </Space>
        <Progress
          style={{ marginTop: 24 }}
          percent={Math.min(100, (event.streak_days / event.required_streak_days) * 100)}
          format={() => `连续 ${event.streak_days}/${event.required_streak_days} 天`}
        />
        <Button
          type="primary"
          size="large"
          disabled={!open || !event.eligible || event.claimed || event.remaining_slots <= 0}
          loading={mutation.isPending}
          onClick={() => mutation.mutate()}
        >
          {event.claimed ? '今日已领取' : '立即领取'}
        </Button>
        {!event.eligible && (
          <Alert
            style={{ marginTop: 16 }}
            type="info"
            message={`连续记录天数不足（活动当天需在 ${opensLabel} 前完成）`}
          />
        )}
        {!event.claimed && event.remaining_slots <= 0 && (
          <Alert style={{ marginTop: 16 }} type="warning" message="本场名额已领完" />
        )}
        {event.claimed && (
          <Alert style={{ marginTop: 16 }} type="success" message="本场活动已经领取" />
        )}
      </Card>
      <Card title="近期成功记录（完全匿名）">
        <List
          locale={{ emptyText: '暂无成功记录' }}
          dataSource={history || []}
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
