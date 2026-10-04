import { useEffect, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Alert, Button, Card, List, Space, message } from 'antd';
import { listDrafts, useDraftScope } from '../../app/drafts';
import { listKnowledge, retryKnowledge } from '../../api/knowledge';
import {
  listScheduledReports,
  listScheduledReportRuns,
  retryScheduledReport,
} from '../../api/scheduledReports';
import { listNotes } from '../../api/notes';
import dayjs from 'dayjs';

export default function PendingWork() {
  const scope = useDraftScope();
  const [drafts, setDrafts] = useState(() => listDrafts(scope));
  useEffect(() => {
    const refresh = () => setDrafts(listDrafts(scope));
    refresh();
    window.addEventListener('cortex:drafts', refresh);
    return () => window.removeEventListener('cortex:drafts', refresh);
  }, [scope]);
  const documents = useQuery({ queryKey: ['knowledge'], queryFn: listKnowledge });
  const failures = useQuery({
    queryKey: ['scheduled-failures'],
    queryFn: async () => {
      const tasks = await listScheduledReports();
      // Bound request fan-out for accounts with many schedules.
      const runs = await Promise.all(
        tasks
          .slice(0, 20)
          .map(async (task) => ({ task, run: (await listScheduledReportRuns(task.id))[0] })),
      );
      return runs.filter((item) => item.run?.status === 'failed');
    },
  });
  const lastWeek = dayjs().subtract(7, 'day').format('YYYY-MM-DD');
  const review = useQuery({
    queryKey: ['review', lastWeek],
    queryFn: () => listNotes({ start_date: lastWeek, end_date: lastWeek, page_size: 5 }),
  });
  const failedDocuments =
    documents.data?.items?.filter(
      (item) => item.Status === 'failed' || item.index_job_status === 'failed',
    ) || [];
  return (
    <>
      <Card title="待处理" style={{ marginTop: 16 }}>
        {(documents.isError || failures.isError) && (
          <Alert
            type="warning"
            message="部分任务状态加载失败"
            action={
              <Button
                onClick={() => {
                  void documents.refetch();
                  void failures.refetch();
                }}
              >
                重试加载
              </Button>
            }
          />
        )}
        <List
          dataSource={drafts}
          locale={{ emptyText: '当前标签页没有待确认草稿' }}
          renderItem={(draft) => (
            <List.Item>
              <a href={draft.path}>继续：{draft.label}</a>
            </List.Item>
          )}
        />
        <List
          dataSource={failedDocuments}
          locale={{ emptyText: '没有索引失败文件' }}
          renderItem={(doc) => (
            <List.Item
              actions={[
                <Button
                  key="retry"
                  onClick={async () => {
                    try {
                      await retryKnowledge(doc.id);
                      await documents.refetch();
                      message.success('索引已加入重试队列');
                    } catch {
                      message.error('重试失败');
                    }
                  }}
                >
                  重试
                </Button>,
              ]}
            >
              <a href="/knowledge">
                {doc.Title} ·{' '}
                {doc.failure_summary ||
                  doc.last_index_failure_code ||
                  doc.failure_code ||
                  '索引失败'}
              </a>
            </List.Item>
          )}
        />
        <List
          dataSource={failures.data || []}
          locale={{ emptyText: '最近检查的定时任务没有失败' }}
          renderItem={({ task, run }) => (
            <List.Item
              actions={[
                <Button
                  key="retry"
                  onClick={async () => {
                    try {
                      await retryScheduledReport(task.id);
                      message.success('已加入执行队列');
                      await failures.refetch();
                    } catch {
                      message.error('重试失败');
                    }
                  }}
                >
                  重试
                </Button>,
              ]}
            >
              <a href="/reports">
                {task.report_type} · {run.error_message || run.error_code || '报告生成失败'}
              </a>
            </List.Item>
          )}
        />
        <Space>
          <a href="/reports">查看全部定时任务</a>
          <span>草稿保存在当前浏览器标签页，保存到服务器后才进入笔记。</span>
        </Space>
      </Card>
      <Card title="上周今天" style={{ marginTop: 16 }}>
        {review.isError && <Alert type="warning" message="回顾加载失败" />}
        <List
          dataSource={review.data?.items || []}
          locale={{ emptyText: '上周今天没有记录' }}
          renderItem={(note) => (
            <List.Item>
              <a href={`/notes/${note.id}`}>{note.title}</a>
            </List.Item>
          )}
        />
      </Card>
    </>
  );
}
