import { useState } from 'react';
import { Alert, Button, Card, Input, Space, message } from 'antd';
import ReactMarkdown from 'react-markdown';
import { confirmOrganize, streamPost } from '../../api/m2';
export default function DashboardPage({ token }: { token: string }) {
  const [raw, setRaw] = useState('');
  const [draft, setDraft] = useState('');
  const [loading, setLoading] = useState(false);
  const [preview, setPreview] = useState<{
    title: string;
    summary?: string;
    content: string;
  } | null>(null);
  async function organize() {
    setLoading(true);
    setDraft('');
    setPreview(null);
    let out = '';
    try {
      await streamPost(token, '/ai/organize', { content: raw }, (c) => {
        out += c;
        setDraft(out);
      });
      const clean = out.replace(/^```json\s*|\s*```$/g, '');
      setPreview(JSON.parse(clean));
    } catch (e) {
      message.error(e instanceof Error ? e.message : '整理失败');
    } finally {
      setLoading(false);
    }
  }
  async function save() {
    if (!preview) return;
    try {
      const n = await confirmOrganize(token, preview);
      message.success(`已保存笔记 #${n.id}`);
      setRaw('');
      setDraft('');
      setPreview(null);
    } catch (e) {
      message.error(e instanceof Error ? e.message : '保存失败');
    }
  }
  return (
    <div className="feature-page">
      <h1>快速记录</h1>
      <Card>
        <Input.TextArea
          value={raw}
          onChange={(e) => setRaw(e.target.value)}
          rows={8}
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
                onChange={(e) => setPreview({ ...preview, title: e.target.value })}
              />
              <Input.TextArea
                style={{ marginTop: 12 }}
                rows={12}
                value={preview.content}
                onChange={(e) => setPreview({ ...preview, content: e.target.value })}
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
    </div>
  );
}
