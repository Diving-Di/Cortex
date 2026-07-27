import { useRef, useState } from 'react';
import { Button, Card, Empty, Input, Space, Spin, Typography, message } from 'antd';
import { RobotOutlined, SendOutlined, StopOutlined, UserOutlined } from '@ant-design/icons';
import './GrowthAssistantPage.css';

interface Props {
  token: string;
}

interface ChatItem {
  id: number;
  role: 'user' | 'assistant';
  content: string;
}

export default function GrowthAssistantPage({ token }: Props) {
  const [items, setItems] = useState<ChatItem[]>([]);
  const [input, setInput] = useState('');
  const [sending, setSending] = useState(false);
  const controller = useRef<AbortController>();

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
        body: JSON.stringify({ question, collection_ids: [], document_ids: [] }),
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
        for (const event of events) {
          for (const line of event.split('\n')) {
            if (!line.startsWith('data: ')) continue;
            const data = line.slice(6);
            if (data === '[DONE]') continue;
            try {
              const parsed = JSON.parse(data) as { content?: string; code?: string };
              if (parsed.code) throw new Error(parsed.code);
              if (parsed.content) {
                setItems((current) =>
                  current.map((item) =>
                    item.id === assistantId
                      ? { ...item, content: item.content + parsed.content }
                      : item,
                  ),
                );
              }
            } catch (error) {
              if ((error as Error).message !== 'Unexpected end of JSON input') throw error;
            }
          }
        }
      }
    } catch (error) {
      if ((error as Error).name !== 'AbortError') {
        message.error('知识问答失败，请确认文件已经完成索引');
        setItems((current) =>
          current.map((item) =>
            item.id === assistantId && !item.content
              ? { ...item, content: '没有找到足够的知识依据，或服务暂时不可用。' }
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
    <div className="growth-assistant">
      <div>
        <Typography.Title level={2}>成长助手</Typography.Title>
        <Typography.Text type="secondary">回答严格依据你维护的个人知识库。</Typography.Text>
      </div>
      <Card className="growth-chat-card">
        <div className="growth-messages">
          {items.length === 0 ? (
            <Empty description="上传并完成索引后，可以开始提问" />
          ) : (
            items.map((item) => (
              <div key={item.id} className={`growth-message ${item.role}`}>
                <span className="growth-avatar">
                  {item.role === 'user' ? <UserOutlined /> : <RobotOutlined />}
                </span>
                <div className="growth-bubble">
                  {item.role === 'assistant' && !item.content ? (
                    <Spin size="small" />
                  ) : (
                    item.content
                  )}
                </div>
              </div>
            ))
          )}
        </div>
        <Space.Compact className="growth-input">
          <Input.TextArea
            value={input}
            onChange={(event) => setInput(event.target.value)}
            onPressEnter={(event) => {
              if (!event.shiftKey) {
                event.preventDefault();
                void send();
              }
            }}
            placeholder="询问你的知识库，Shift+Enter 换行"
            autoSize={{ minRows: 1, maxRows: 5 }}
          />
          {sending ? (
            <Button icon={<StopOutlined />} onClick={() => controller.current?.abort()}>
              停止
            </Button>
          ) : (
            <Button type="primary" icon={<SendOutlined />} onClick={() => void send()}>
              发送
            </Button>
          )}
        </Space.Compact>
      </Card>
    </div>
  );
}
