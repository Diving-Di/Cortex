import { useEffect, useRef, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  Alert,
  Button,
  Card,
  Empty,
  Popconfirm,
  Progress,
  Input,
  List,
  Collapse,
  Select,
  Space,
  Table,
  Tag,
  Typography,
  Upload,
  message,
} from 'antd';
import { DeleteOutlined, InboxOutlined, SendOutlined, StopOutlined } from '@ant-design/icons';
import {
  deleteKnowledge,
  getKnowledgeConversation,
  listKnowledge,
  listKnowledgeConversations,
  sendKnowledgeFeedback,
  streamKnowledge,
  uploadKnowledge,
  type KnowledgeSource,
  KnowledgeStreamError,
  type RetrievalProgress,
} from '../../api/knowledge';
import './KnowledgePage.css';

const statusText: Record<string, string> = {
  uploaded: '已上传',
  parsing: '解析中',
  indexing: '索引中',
  ready: '可用',
  failed: '失败',
  deleting: '删除中',
};
export default function KnowledgePage() {
  const queryClient = useQueryClient();
  const query = useQuery({
    queryKey: ['knowledge'],
    queryFn: () => listKnowledge(),
    refetchInterval: 5000,
  });
  const conversations = useQuery({
    queryKey: ['knowledge-conversations'],
    queryFn: () => listKnowledgeConversations(),
  });
  const upload = useMutation({
    mutationFn: (file: File) => uploadKnowledge(file),
    onSuccess: () => {
      message.success('文件已安全保存，正在建立索引');
      queryClient.invalidateQueries({ queryKey: ['knowledge'] });
    },
    onError: () => message.error('上传失败，请检查文件格式和容量'),
  });
  const remove = useMutation({
    mutationFn: (id: string) => deleteKnowledge(id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['knowledge'] }),
  });
  const quota = query.data?.quota;
  const percent = quota ? Math.min(100, (quota.used_bytes / quota.limit_bytes) * 100) : 0;
  const [question, setQuestion] = useState('');
  const [answer, setAnswer] = useState('');
  const [sources, setSources] = useState<KnowledgeSource[]>([]);
  const [stages, setStages] = useState<RetrievalProgress[]>([]);
  const [conversationID, setConversationID] = useState<number>();
  const [requestID, setRequestID] = useState('');
  const [streaming, setStreaming] = useState(false);
  const [incomplete, setIncomplete] = useState(false);
  const [clarification, setClarification] = useState<{
    id: string;
    prompt: string;
    value: string;
  }>();
  const abortRef = useRef<AbortController>();
  useEffect(() => () => abortRef.current?.abort(), []);

  const ask = async (resume?: { id: string; value: string }) => {
    const value = question.trim();
    if (!value || streaming) return;
    const id = crypto.randomUUID();
    const controller = new AbortController();
    abortRef.current = controller;
    setRequestID(id);
    setAnswer('');
    setSources([]);
    setStages([]);
    setIncomplete(false);
    setStreaming(true);
    try {
      await streamKnowledge(
        {
          question: value,
          request_id: id,
          conversation_id: conversationID,
          resume_clarification_id: resume?.id,
          clarification: resume?.value,
        },
        (event) => {
          if (event.type === 'retrieval_progress')
            setStages((old) => [...old.filter((v) => v.stage !== event.data.stage), event.data]);
          if (event.type === 'delta' || event.type === 'verified')
            setAnswer((old) => old + event.data.content);
          if (event.type === 'retrieval') setSources(event.data.items);
          if (event.type === 'sources')
            setSources(Array.isArray(event.data) ? event.data : event.data.items);
          if (event.type === 'done') {
            setConversationID(event.data.conversation_id);
            void queryClient.invalidateQueries({ queryKey: ['knowledge-conversations'] });
          }
          if (event.type === 'error') {
            setIncomplete(Boolean(event.data.incomplete));
            if (event.data.conversation_id) setConversationID(event.data.conversation_id);
            message.error(event.data.message || '回答未完成');
          }
        },
        controller.signal,
      );
    } catch (error) {
      if (!controller.signal.aborted) {
        if (
          error instanceof KnowledgeStreamError &&
          error.code === 'KNOWLEDGE_CLARIFICATION_REQUIRED' &&
          error.details
        ) {
          setConversationID(error.details.conversation_id);
          setClarification({
            id: error.details.clarification_id,
            prompt: error.details.prompt,
            value: '',
          });
        } else {
          message.error(error instanceof Error ? error.message : '知识问答失败');
        }
      }
    } finally {
      setStreaming(false);
    }
  };
  return (
    <div className="knowledge-page">
      <Typography.Title level={2}>个人知识库</Typography.Title>
      <Alert
        showIcon
        type="info"
        message="仅支持 Markdown 或 Markdown ZIP"
        description="ZIP 仅解析 Markdown、PNG 和 JPG，其他类型条目会被跳过。所有资料只对当前账号可见，每个账号上限 3 GiB。"
      />
      <Card
        title="知识问答"
        extra={
          <Select
            allowClear
            showSearch
            style={{ width: 220 }}
            placeholder="新会话 / 选择历史会话"
            value={conversationID}
            options={(conversations.data?.items ?? []).map((item) => ({
              value: item.id,
              label: item.title,
            }))}
            onClear={() => {
              setConversationID(undefined);
              setAnswer('');
              setSources([]);
            }}
            onChange={(id) => {
              setConversationID(id);
              void getKnowledgeConversation(id).then((detail) => {
                const last = [...detail.messages]
                  .reverse()
                  .find((item) => item.role === 'assistant');
                setAnswer(last?.content ?? '');
                setSources([]);
                setIncomplete(last?.status === 'failed');
              });
            }}
          />
        }
      >
        <Space.Compact block>
          <Input.TextArea
            value={question}
            autoSize={{ minRows: 2, maxRows: 5 }}
            maxLength={5000}
            placeholder="根据我的知识库提问"
            onChange={(event) => setQuestion(event.target.value)}
            onPressEnter={(event) => {
              if (!event.shiftKey) {
                event.preventDefault();
                void ask();
              }
            }}
          />
          {streaming ? (
            <Button danger icon={<StopOutlined />} onClick={() => abortRef.current?.abort()}>
              停止
            </Button>
          ) : (
            <Button type="primary" icon={<SendOutlined />} onClick={() => void ask()}>
              发送
            </Button>
          )}
        </Space.Compact>
        {stages.length > 0 && (
          <div className="knowledge-trace" aria-label="检索过程">
            <Space wrap>
              {stages.map((stage) => (
                <Tag
                  key={stage.stage}
                  color={stage.status === 'degraded' ? 'warning' : 'processing'}
                >
                  {stage.stage} · {stage.elapsed_ms} ms
                </Tag>
              ))}
            </Space>
          </div>
        )}
        {clarification && (
          <Alert
            className="knowledge-answer"
            type="info"
            showIcon
            message={clarification.prompt}
            description={
              <Space.Compact block>
                <Input
                  maxLength={1000}
                  value={clarification.value}
                  placeholder="补充一次后继续原问题"
                  onChange={(event) =>
                    setClarification({ ...clarification, value: event.target.value })
                  }
                />
                <Button
                  type="primary"
                  disabled={!clarification.value.trim() || streaming}
                  onClick={() => {
                    const pending = clarification;
                    setClarification(undefined);
                    void ask(pending);
                  }}
                >
                  继续检索
                </Button>
              </Space.Compact>
            }
          />
        )}
        {(answer || incomplete) && (
          <Alert
            className="knowledge-answer"
            type={incomplete ? 'warning' : 'success'}
            message={incomplete ? '回答不完整' : '回答'}
            description={
              <Typography.Paragraph style={{ whiteSpace: 'pre-wrap' }}>
                {answer || '生成在完成前中断'}
              </Typography.Paragraph>
            }
          />
        )}
        {sources.length > 0 && (
          <Collapse
            className="knowledge-sources"
            items={[
              {
                key: 'sources',
                label: `来源（${sources.length}）`,
                children: (
                  <List
                    dataSource={sources}
                    renderItem={(source) => (
                      <List.Item>
                        <Typography.Text>
                          <Tag>{source.citation}</Tag>
                          {source.title}
                          {source.heading.length ? ` · ${source.heading.join(' / ')}` : ''}
                        </Typography.Text>
                      </List.Item>
                    )}
                  />
                ),
              },
            ]}
          />
        )}
        {requestID && answer && !streaming && (
          <Space className="knowledge-feedback">
            <Typography.Text type="secondary">这个回答有问题？</Typography.Text>
            <Button
              size="small"
              onClick={() =>
                void sendKnowledgeFeedback(requestID, 'incorrect_answer').then(() =>
                  message.success('反馈已记录'),
                )
              }
            >
              答案不正确
            </Button>
            <Button
              size="small"
              onClick={() =>
                void sendKnowledgeFeedback(requestID, 'unsupported_citation').then(() =>
                  message.success('反馈已记录'),
                )
              }
            >
              引用无依据
            </Button>
          </Space>
        )}
      </Card>
      <Card title="上传资料">
        <Upload.Dragger
          accept=".md,.zip"
          multiple={false}
          showUploadList={false}
          disabled={upload.isPending}
          beforeUpload={(file) => {
            upload.mutate(file);
            return false;
          }}
        >
          <p className="ant-upload-drag-icon">
            <InboxOutlined />
          </p>
          <p>拖放或点击选择 .md / .zip 文件</p>
        </Upload.Dragger>
        {quota && (
          <div className="knowledge-quota">
            <Space>
              <span>已用 {(quota.used_bytes / 1024 / 1024).toFixed(1)} MiB</span>
              <span>剩余 {(quota.remaining_bytes / 1024 / 1024).toFixed(1)} MiB</span>
            </Space>
            <Progress percent={Number(percent.toFixed(1))} showInfo={false} />
          </div>
        )}
      </Card>
      <Card title="知识文档">
        <Table
          rowKey="id"
          loading={query.isLoading}
          dataSource={query.data?.items ?? []}
          locale={{ emptyText: <Empty description="还没有知识文档" /> }}
          columns={[
            { title: '标题', dataIndex: 'Title' },
            {
              title: '来源',
              dataIndex: 'SourceType',
              render: (v) => (v === 'note' ? '个人笔记' : '上传资料'),
            },
            {
              title: '大小',
              dataIndex: 'size_bytes',
              render: (v) => `${(v / 1024).toFixed(1)} KiB`,
            },
            {
              title: '状态',
              dataIndex: 'Status',
              render: (v, row) => (
                <Space direction="vertical" size={0}>
                  <Tag color={v === 'ready' ? 'success' : v === 'failed' ? 'error' : 'processing'}>
                    {statusText[v] ?? v}
                  </Tag>
                  {v === 'ready' && ['queued', 'running'].includes(row.index_job_status ?? '') && (
                    <Typography.Text type="secondary">可用，正在更新索引</Typography.Text>
                  )}
                  {v === 'ready' && row.index_job_status === 'failed' && (
                    <Typography.Text type="warning">
                      旧版本可用，最近更新失败：
                      {row.last_index_failure_code ?? 'KNOWLEDGE_INDEX_FAILED'}
                    </Typography.Text>
                  )}
                  {row.failure_summary && (
                    <Typography.Text type="danger">{row.failure_summary}</Typography.Text>
                  )}
                  {row.index_stage &&
                    ['queued', 'running'].includes(row.index_job_status ?? '') && (
                      <div className="knowledge-index-progress">
                        <Typography.Text type="secondary">{row.index_stage}</Typography.Text>
                        <Progress
                          size="small"
                          percent={
                            row.total_chunks
                              ? Math.round(((row.processed_chunks ?? 0) / row.total_chunks) * 100)
                              : 0
                          }
                          showInfo={Boolean(row.total_chunks)}
                          status="active"
                        />
                      </div>
                    )}
                </Space>
              ),
            },
            {
              title: '操作',
              render: (_, row) => (
                <Popconfirm title="删除此知识文档？" onConfirm={() => remove.mutate(row.id)}>
                  <Button danger type="text" icon={<DeleteOutlined />}>
                    删除
                  </Button>
                </Popconfirm>
              ),
            },
          ]}
        />
      </Card>
    </div>
  );
}
