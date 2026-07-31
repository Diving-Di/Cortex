import { useCallback, useEffect, useRef, useState } from 'react';
import { HistoryOutlined, PlusOutlined, SendOutlined, StopOutlined } from '@ant-design/icons';
import { Alert, Button, Empty, Input, List, Spin, Typography, message } from 'antd';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import {
  getRecipeConversation,
  getTodayRecipe,
  listRecipeConversations,
  type RecipeConversation,
  type TodayRecipe,
} from '../../api/recipes';
import './TodayRecipePage.css';

interface ChatItem {
  id: number;
  role: 'user' | 'assistant';
  content: string;
}

function newRequestID() {
  return globalThis.crypto?.randomUUID?.() || `request-${Date.now()}-${Math.random()}`;
}

export default function TodayRecipePage({ token }: { token: string }) {
  const [loading, setLoading] = useState(true);
  const [result, setResult] = useState<TodayRecipe | null>(null);
  const [error, setError] = useState('');
  const [conversations, setConversations] = useState<RecipeConversation[]>([]);
  const [conversationId, setConversationId] = useState<number>();
  const [items, setItems] = useState<ChatItem[]>([]);
  const [input, setInput] = useState('');
  const [sending, setSending] = useState(false);
  const controller = useRef<AbortController>();
  const messagesEnd = useRef<HTMLDivElement>(null);

  const loadConversations = useCallback(async () => {
    try {
      setConversations(await listRecipeConversations(token));
    } catch {
      message.error('无法加载聊天记录');
    }
  }, [token]);

  const load = useCallback(() => {
    setLoading(true);
    setError('');
    void Promise.all([getTodayRecipe(token), loadConversations()])
      .then(([recipe]) => setResult(recipe))
      .catch(() => setError('今日菜谱暂时不可用，请稍后重试。'))
      .finally(() => setLoading(false));
  }, [loadConversations, token]);

  useEffect(load, [load]);
  useEffect(() => {
    if (typeof messagesEnd.current?.scrollIntoView === 'function') {
      messagesEnd.current.scrollIntoView({ block: 'end' });
    }
  }, [items]);

  function startNewChat() {
    controller.current?.abort();
    setConversationId(undefined);
    setItems([]);
    setInput('');
  }

  async function openConversation(id: number) {
    try {
      const conversation = await getRecipeConversation(token, id);
      if (conversation.source_scope !== 'recipe') throw new Error('invalid scope');
      setConversationId(id);
      setItems(
        (conversation.messages || []).map((item) => ({
          id: item.id,
          role: item.role,
          content: item.content,
        })),
      );
    } catch {
      message.error('无法打开该聊天');
    }
  }

  const send = useCallback(
    async (value = input) => {
      const question = value.trim();
      if (!question || !result || sending) return;
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
        const response = await fetch('/api/v1/recipes/chat', {
          method: 'POST',
          headers: { Authorization: `Token ${token}`, 'Content-Type': 'application/json' },
          body: JSON.stringify({
            question,
            request_id: newRequestID(),
            conversation_id: conversationId,
            featured_recipe_id: result.recipe.id,
          }),
          signal: controller.current.signal,
        });
        if (!response.ok || !response.body) throw new Error(`HTTP ${response.status}`);
        const reader = response.body.getReader();
        const decoder = new TextDecoder();
        let buffer = '';
        while (true) {
          const { value: chunk, done } = await reader.read();
          if (done) break;
          buffer += decoder.decode(chunk, { stream: true });
          const blocks = buffer.split('\n\n');
          buffer = blocks.pop() || '';
          for (const block of blocks) {
            const event = block
              .split('\n')
              .find((line) => line.startsWith('event: '))
              ?.slice(7);
            const data = block
              .split('\n')
              .find((line) => line.startsWith('data: '))
              ?.slice(6);
            if (!data) continue;
            const payload = JSON.parse(data) as {
              content?: string;
              code?: string;
              conversation_id?: number;
            };
            if (event === 'error') throw new Error(payload.code || 'RECIPE_CHAT_FAILED');
            if (event === 'delta' && payload.content) {
              setItems((current) =>
                current.map((item) =>
                  item.id === assistantId
                    ? { ...item, content: item.content + payload.content }
                    : item,
                ),
              );
            }
            if (event === 'done' && payload.conversation_id) {
              setConversationId(payload.conversation_id);
              void loadConversations();
            }
          }
        }
      } catch (requestError) {
        if ((requestError as Error).name !== 'AbortError') {
          message.error('菜谱问答暂时不可用');
          setItems((current) =>
            current.map((item) =>
              item.id === assistantId && !item.content
                ? { ...item, content: '没有找到足够的菜谱依据，或服务暂时不可用。' }
                : item,
            ),
          );
        }
      } finally {
        setSending(false);
        controller.current = undefined;
      }
    },
    [conversationId, input, loadConversations, result, sending, token],
  );

  if (loading) return <Spin aria-label="正在挑选今日菜谱" />;
  if (error || !result)
    return (
      <Alert
        type="error"
        message={error || '未能加载今日菜谱。'}
        action={<Button onClick={load}>重试</Button>}
        showIcon
      />
    );

  return (
    <div className="recipe-workspace">
      <aside className="recipe-history" aria-label="聊天记录">
        <div className="recipe-history-title">
          <HistoryOutlined />
          <Typography.Title level={4}>聊天记录</Typography.Title>
        </div>
        <Button block icon={<PlusOutlined />} onClick={startNewChat}>
          新聊天
        </Button>
        <List
          dataSource={conversations}
          locale={{ emptyText: '暂无聊天记录' }}
          renderItem={(conversation) => (
            <List.Item
              className={conversation.id === conversationId ? 'active' : ''}
              onClick={() => void openConversation(conversation.id)}
            >
              <List.Item.Meta
                title={conversation.title}
                description={`${conversation.message_count || 0} 条消息`}
              />
            </List.Item>
          )}
        />
      </aside>

      <main className="recipe-chat">
        <header className="recipe-hero">
          <Typography.Title level={2}>今日菜谱：{result.recipe.title}</Typography.Title>
          <Typography.Paragraph type="secondary">{result.recipe.summary}</Typography.Paragraph>
          <Typography.Text type="secondary">
            <strong>食材：</strong>
            {result.recipe.ingredients.join('、')}
          </Typography.Text>
        </header>

        <section className="recipe-messages" aria-live="polite">
          {items.length === 0 ? (
            <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="询问这道菜的做法和技巧" />
          ) : (
            items.map((item) => (
              <div key={item.id} className={`recipe-message ${item.role}`}>
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
            ))
          )}
          <div ref={messagesEnd} />
        </section>

        <footer className="recipe-composer-area">
          <div className="recipe-composer">
            <Input.TextArea
              aria-label="烹饪问题"
              value={input}
              onChange={(event) => setInput(event.target.value)}
              onPressEnter={(event) => {
                if (!event.shiftKey) {
                  event.preventDefault();
                  void send();
                }
              }}
              placeholder="询问今日菜谱，Shift+Enter 换行"
              autoSize={{ minRows: 2, maxRows: 5 }}
            />
            {sending ? (
              <Button
                aria-label="停止生成"
                icon={<StopOutlined />}
                shape="circle"
                onClick={() => controller.current?.abort()}
              />
            ) : (
              <Button
                type="primary"
                aria-label="发送"
                icon={<SendOutlined />}
                shape="circle"
                disabled={!input.trim()}
                onClick={() => void send()}
              />
            )}
          </div>
          <div className="recipe-suggestions" aria-label="建议问题">
            {result.suggested_questions.map((question) => (
              <Button key={question} onClick={() => void send(question)}>
                {question}
              </Button>
            ))}
          </div>
        </footer>
      </main>
    </div>
  );
}
