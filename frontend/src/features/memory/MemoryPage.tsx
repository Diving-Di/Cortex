import { useState } from 'react';
import { Alert, Button, Card, Input, message } from 'antd';
import ReactMarkdown from 'react-markdown';
import { streamPost } from '../../api/m2';
export default function MemoryPage({ token }: { token: string }) {
  const [q, setQ] = useState('');
  const [answer, setAnswer] = useState('');
  const [loading, setLoading] = useState(false);
  async function ask() {
    setLoading(true);
    setAnswer('');
    let out = '';
    try {
      await streamPost(token, '/memory/chat', { question: q }, (c) => {
        out += c;
        setAnswer(out);
      });
    } catch (e) {
      message.error(e instanceof Error ? e.message : '查询失败');
    } finally {
      setLoading(false);
    }
  }
  return (
    <div className="feature-page">
      <h1>回忆书</h1>
      <Card>
        <Input.Search
          value={q}
          onChange={(e) => setQ(e.target.value)}
          onSearch={ask}
          enterButton={<Button loading={loading}>查找回忆</Button>}
          placeholder="例如：上周我在杭州做了什么？"
        />
      </Card>
      {answer ? (
        <Card title="基于笔记的回答" style={{ marginTop: 16 }}>
          <ReactMarkdown
            components={{
              a: ({ href, children }) => <a href={href}>{children}</a>,
            }}
          >
            {answer.replace(/\[#(\d+)\]/g, '[$&](/notes/$1)')}
          </ReactMarkdown>
        </Card>
      ) : (
        <Alert
          style={{ marginTop: 16 }}
          message={
            <span style={{ color: '#fff' }}>
              回答只使用当前个人空间中的笔记；没有证据时会明确提示。
            </span>
          }
        />
      )}
    </div>
  );
}
