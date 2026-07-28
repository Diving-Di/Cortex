import { useEffect, useRef, useState } from 'react';
import {
  Button,
  Card,
  Empty,
  Input,
  List,
  Modal,
  Popconfirm,
  Select,
  Spin,
  Typography,
  message,
} from 'antd';
import {
  DeleteOutlined,
  EditOutlined,
  MenuFoldOutlined,
  MenuUnfoldOutlined,
  PlusOutlined,
  RobotOutlined,
  SendOutlined,
  StopOutlined,
  UserOutlined,
} from '@ant-design/icons';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import {
  deleteConversation,
  getConversation,
  listConversations,
  renameConversation,
} from '../../api/knowledge';
import type { Conversation } from '../../api/knowledge';
import './GrowthAssistantPage.css';

interface Props {
  token: string;
}

interface Source {
  source_type: 'knowledge_document' | 'growth_note';
  source_id: number;
  title: string;
  snippet?: string;
  heading?: string;
  page_from?: number;
  page_to?: number;
  source_deleted?: boolean;
}

interface ChatItem {
  id: number;
  role: 'user' | 'assistant';
  content: string;
  sources?: Source[];
}

function newRequestID() {
  return globalThis.crypto?.randomUUID?.() || `request-${Date.now()}-${Math.random()}`;
}

export default function GrowthAssistantPage({ token }: Props) {
  const queryClient = useQueryClient();
  const [items, setItems] = useState<ChatItem[]>([]);
  const [input, setInput] = useState('');
  const [sending, setSending] = useState(false);
  const [scope, setScope] = useState<Conversation['source_scope']>('knowledge');
  const [conversationId, setConversationId] = useState<number>();
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);
  const controller = useRef<AbortController>();
  const messagesEnd = useRef<HTMLDivElement>(null);
  const conversations = useQuery({
    queryKey: ['assistant-conversations'],
    queryFn: () => listConversations(token),
  });

  useEffect(() => {
    if (typeof messagesEnd.current?.scrollIntoView === 'function') {
      messagesEnd.current.scrollIntoView({ block: 'end' });
    }
  }, [items]);

  function startCleanConversation() {
    controller.current?.abort();
    setConversationId(undefined);
    setItems([]);
    setInput('');
  }

  const removeChat = useMutation({
    mutationFn: (id: number) => deleteConversation(token, id),
    onSuccess: async (_, id) => {
      if (conversationId === id) {
        setConversationId(undefined);
        setItems([]);
      }
      await queryClient.invalidateQueries({ queryKey: ['assistant-conversations'] });
    },
  });

  async function openConversation(id: number) {
    try {
      const conversation = await getConversation(token, id);
      setConversationId(id);
      setScope(conversation.source_scope);
      setItems(
        (conversation.messages || []).map((item) => ({
          id: item.id,
          role: item.role,
          content: item.content,
        })),
      );
    } catch {
      message.error('无法打开该会话');
    }
  }

  async function send() {
    const question = input.trim();
    if (!question || sending) return;
    const userItem = { id: Date.now(), role: 'user' as const, content: question };
    const assistantId = userItem.id + 1;
    setItems((current) => [
      ...current,
      userItem,
      { id: assistantId, role: 'assistant', content: '' },
    ]);
    setInput('');
    setSending(true);
    controller.current = new AbortController();
    try {
      const response = await fetch('/api/v1/knowledge/chat', {
        method: 'POST',
        headers: { Authorization: `Token ${token}`, 'Content-Type': 'application/json' },
        body: JSON.stringify({
          question,
          request_id: newRequestID(),
          source_scope: scope,
          conversation_id: conversationId,
        }),
        signal: controller.current.signal,
      });
      if (!response.ok || !response.body) throw new Error(`HTTP ${response.status}`);
      const reader = response.body.getReader();
      const decoder = new TextDecoder();
      let buffer = '';
      while (true) {
        const { value, done } = await reader.read();
        if (done) break;
        buffer += decoder.decode(value, { stream: true });
        const events = buffer.split('\n\n');
        buffer = events.pop() || '';
        for (const block of events) {
          const event = block
            .split('\n')
            .find((line) => line.startsWith('event: '))
            ?.slice(7);
          const data = block
            .split('\n')
            .find((line) => line.startsWith('data: '))
            ?.slice(6);
          if (!data) continue;
          const parsed = JSON.parse(data) as {
            content?: string;
            code?: string;
            items?: Source[];
            conversation_id?: number;
          };
          if (event === 'error') throw new Error(parsed.code || 'AI_REQUEST_FAILED');
          if (event === 'delta' && parsed.content) {
            setItems((current) =>
              current.map((item) =>
                item.id === assistantId
                  ? { ...item, content: item.content + parsed.content }
                  : item,
              ),
            );
          }
          if (event === 'sources') {
            setItems((current) =>
              current.map((item) =>
                item.id === assistantId ? { ...item, sources: parsed.items || [] } : item,
              ),
            );
          }
          if (event === 'done' && parsed.conversation_id) {
            setConversationId(parsed.conversation_id);
            void queryClient.invalidateQueries({ queryKey: ['assistant-conversations'] });
          }
        }
      }
    } catch (error) {
      if ((error as Error).name !== 'AbortError') {
        message.error('问答失败，请检查来源是否可用');
        setItems((current) =>
          current.map((item) =>
            item.id === assistantId && !item.content
              ? { ...item, content: '没有找到足够依据，或服务暂时不可用。' }
              : item,
          ),
        );
      }
    } finally {
      setSending(false);
      controller.current = undefined;
    }
  }

  return (
    <div className={`growth-assistant${sidebarCollapsed ? ' sidebar-collapsed' : ''}`}>
      <aside className="growth-sidebar" aria-label="会话列表">
        <div className="growth-sidebar-header">
          {!sidebarCollapsed ? (
            <Button block icon={<PlusOutlined />} onClick={startCleanConversation}>
              新建会话
            </Button>
          ) : null}
          <Button
            type="text"
            aria-label={sidebarCollapsed ? '展开会话列表' : '折叠会话列表'}
            icon={sidebarCollapsed ? <MenuUnfoldOutlined /> : <MenuFoldOutlined />}
            onClick={() => setSidebarCollapsed((value) => !value)}
          />
        </div>
        {!sidebarCollapsed ? (
          <List
            loading={conversations.isLoading}
            dataSource={conversations.data || []}
            locale={{ emptyText: '暂无会话' }}
            renderItem={(conversation) => (
              <List.Item
                className={conversation.id === conversationId ? 'active' : ''}
                onClick={() => void openConversation(conversation.id)}
                actions={[
                  <Button
                    key="rename"
                    type="text"
                    aria-label={`重命名会话 ${conversation.title}`}
                    icon={<EditOutlined />}
                    onClick={(event) => {
                      event.stopPropagation();
                      Modal.confirm({
                        title: '重命名会话',
                        content: (
                          <Input
                            id={`rename-${conversation.id}`}
                            defaultValue={conversation.title}
                            maxLength={255}
                          />
                        ),
                        onOk: async () => {
                          const title = (
                            document.getElementById(`rename-${conversation.id}`) as HTMLInputElement
                          )?.value.trim();
                          if (!title) return;
                          await renameConversation(
                            token,
                            conversation.id,
                            title,
                            conversation.version,
                          );
                          await queryClient.invalidateQueries({
                            queryKey: ['assistant-conversations'],
                          });
                        },
                      });
                    }}
                  />,
                  <Popconfirm
                    key="delete"
                    title="删除会话？"
                    onConfirm={() => removeChat.mutate(conversation.id)}
                  >
                    <Button
                      type="text"
                      danger
                      aria-label={`删除会话 ${conversation.title}`}
                      icon={<DeleteOutlined />}
                      onClick={(event) => event.stopPropagation()}
                    />
                  </Popconfirm>,
                ]}
              >
                <List.Item.Meta
                  title={conversation.title}
                  description={`${conversation.message_count || 0} 条消息`}
                />
              </List.Item>
            )}
          />
        ) : null}
      </aside>
      <main className="growth-main">
        <div className="growth-source-bar">
          <span>来源</span>
          <Select
            className="growth-source-select"
            aria-label="来源范围"
            value={scope}
            onChange={(value) => {
              setScope(value);
              startCleanConversation();
            }}
            options={[
              { value: 'knowledge', label: '知识库' },
              { value: 'growth', label: '笔记本' },
            ]}
          />
        </div>
        <Card className={`growth-chat-card${items.length === 0 ? ' empty' : ''}`}>
          <div className="growth-messages" aria-live="polite">
            {items.length === 0 ? (
              <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="有什么想了解的？" />
            ) : (
              items.map((item) => (
                <div key={item.id} className={`growth-message ${item.role}`}>
                  <span className="growth-avatar">
                    {item.role === 'user' ? <UserOutlined /> : <RobotOutlined />}
                  </span>
                  <div>
                    <div className="growth-bubble">
                      {item.role === 'assistant' && !item.content ? (
                        <Spin size="small" />
                      ) : item.role === 'assistant' ? (
                        <ReactMarkdown remarkPlugins={[remarkGfm]} skipHtml>
                          {item.content}
                        </ReactMarkdown>
                      ) : (
                        item.content
                      )}
                    </div>
                    {item.sources?.map((source) => (
                      <Card
                        key={`${source.source_type}-${source.source_id}`}
                        size="small"
                        className="source-card"
                      >
                        <Typography.Text strong>{source.title}</Typography.Text>
                        <Typography.Text type={source.source_deleted ? 'danger' : 'secondary'}>
                          {source.source_deleted
                            ? '来源已删除，无法查看或下载'
                            : `${source.heading || ''}${source.page_from ? ` · 第 ${source.page_from} 页` : ''}`}
                        </Typography.Text>
                        {!source.source_deleted && source.snippet ? (
                          <Typography.Paragraph ellipsis={{ rows: 3 }}>
                            {source.snippet}
                          </Typography.Paragraph>
                        ) : null}
                      </Card>
                    ))}
                  </div>
                </div>
              ))
            )}
            <div ref={messagesEnd} />
          </div>
          <div className="growth-input">
            <Input.TextArea
              aria-label="输入问题"
              value={input}
              onChange={(event) => setInput(event.target.value)}
              onPressEnter={(event) => {
                if (!event.shiftKey) {
                  event.preventDefault();
                  void send();
                }
              }}
              placeholder="询问你的资料，Shift+Enter 换行"
              autoSize={{ minRows: 1, maxRows: 4 }}
            />
            {sending ? (
              <Button
                className="growth-send"
                aria-label="停止生成"
                icon={<StopOutlined />}
                onClick={() => controller.current?.abort()}
              />
            ) : (
              <Button
                className="growth-send"
                type="primary"
                aria-label="发送"
                icon={<SendOutlined />}
                onClick={() => void send()}
              />
            )}
          </div>
        </Card>
      </main>
    </div>
  );
}
