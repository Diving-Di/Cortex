import React, { useCallback, useEffect, useState } from 'react';
import { Alert, Button, Card, Input, List, Space, Spin, Typography, message } from 'antd';
import { getTodayRecipe, type TodayRecipe } from '../../api/recipes';

export default function TodayRecipePage({ token }: { token: string }) {
  const [loading, setLoading] = useState(true);
  const [result, setResult] = useState<TodayRecipe | null>(null);
  const [error, setError] = useState('');
  const [question, setQuestion] = useState('');
  const [answer, setAnswer] = useState('');
  const [asking, setAsking] = useState(false);

  const load = useCallback(() => {
    setLoading(true)
    setError('');
    void getTodayRecipe(token)
      .then(setResult)
      .catch(() => setError('今日菜谱暂时不可用，请稍后重试。'))
      .finally(() => setLoading(false));
  }, [token]);

  useEffect(load, [load]);

  const ask = useCallback(
    async (value: string) => {
      const trimmed = value.trim();
      if (!trimmed || !result || asking) return;
      setQuestion(trimmed);
      setAnswer('');
      setAsking(true);
      try {
        const response = await fetch('/api/v1/recipes/chat', {
          method: 'POST',
          headers: { Authorization: `Token ${token}`, 'Content-Type': 'application/json' },
          body: JSON.stringify({
            question: trimmed,
            request_id: crypto.randomUUID(),
            featured_recipe_id: result.recipe.id,
          }),
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
            const payload = JSON.parse(data) as { content?: string; code?: string };
            if (event === 'error') throw new Error(payload.code || 'RECIPE_CHAT_FAILED');
            if (event === 'delta' && payload.content) {
              setAnswer((current) => current + payload.content);
            }
          }
        }
      } catch {
        message.error('菜谱问答暂时不可用');
        setAnswer('没有找到足够的菜谱依据，或服务暂时不可用。');
      } finally {
        setAsking(false);
      }
    },
    [asking, result, token],
  );

  if (loading) return <Spin tip="正在挑选今日菜谱" />;
  if (error || !result)
    return (
      <Card title="今日菜谱">
        <Alert type="error" message={error || '未能加载今日菜谱。'} showIcon />
        <Button onClick={load} style={{ marginTop: 16 }}>
          重试
        </Button>
      </Card>
    );

  return (
    <div>
      <Card title={`今日菜谱：${result.recipe.title}`} style={{ marginBottom: 16 }}>
        <Typography.Paragraph>
          <strong>分类：</strong>
          {result.recipe.category}
        </Typography.Paragraph>
        <Typography.Paragraph>{result.recipe.summary}</Typography.Paragraph>
        <Typography.Title level={5}>食材</Typography.Title>
        <List<string>
          dataSource={result.recipe.ingredients}
          locale={{ emptyText: '请在下方询问完整食材和用量' }}
          renderItem={(item) => <List.Item>{item}</List.Item>}
        />
      </Card>
      <Card title="建议问题">
        <List<string>
          dataSource={result.suggested_questions}
          renderItem={(question) => (
            <List.Item>
              <Button type="link" onClick={() => void ask(question)}>
                {question}
              </Button>
            </List.Item>
          )}
        />
      </Card>
      <Card title="烹饪问答" style={{ marginTop: 16 }}>
        <Space.Compact style={{ width: '100%' }}>
          <Input
            aria-label="烹饪问题"
            value={question}
            onChange={(event) => setQuestion(event.target.value)}
            onPressEnter={() => void ask(question)}
            placeholder="也可以输入任意烹饪问题"
          />
          <Button type="primary" loading={asking} onClick={() => void ask(question)}>
            提问
          </Button>
        </Space.Compact>
        {answer ? (
          <Typography.Paragraph style={{ whiteSpace: 'pre-wrap', marginTop: 16 }}>
            {answer}
          </Typography.Paragraph>
        ) : null}
      </Card>
    </div>
  );
}
