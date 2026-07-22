import { useState } from 'react';
import { Alert, Button, Card, DatePicker, Input, List, Select, Space, message } from 'antd';
import dayjs from 'dayjs';
import { confirmReport, previewReport, Source, streamPost } from '../../api/m2';
export default function ReportsPage({ token }: { token: string }) {
  const [type, setType] = useState('weekly');
  const [anchor, setAnchor] = useState(dayjs());
  const [sources, setSources] = useState<Source[]>([]);
  const [content, setContent] = useState('');
  const [loading, setLoading] = useState(false);
  const [title, setTitle] = useState('周期报告');
  async function load() {
    try {
      const p = await previewReport(token, {
        type,
        anchor_date: anchor.format('YYYY-MM-DD'),
      });
      setSources(p.sources);
      if (!p.sources.length) message.warning('所选周期没有来源，不能生成报告');
    } catch (e) {
      message.error(e instanceof Error ? e.message : '加载失败');
    }
  }
  async function generate() {
    setLoading(true);
    setContent('');
    let out = '';
    try {
      await streamPost(
        token,
        '/reports/generate',
        { type, anchor_date: anchor.format('YYYY-MM-DD') },
        (c) => {
          out += c;
          setContent(out);
        },
      );
    } catch (e) {
      message.error(e instanceof Error ? e.message : '生成失败');
    } finally {
      setLoading(false);
    }
  }
  async function save() {
    try {
      await confirmReport(token, {
        type,
        anchor_date: anchor.format('YYYY-MM-DD'),
        title,
        content,
        source_ids: sources.map((s) => s.id),
        overwrite: false,
      });
      message.success('报告已保存');
    } catch (e) {
      message.error(e instanceof Error ? e.message : '保存失败');
    }
  }
  return (
    <div className="feature-page">
      <h1>周期报告</h1>
      <Card>
        <Space wrap>
          <Select
            value={type}
            onChange={setType}
            options={[
              { value: 'daily', label: '日报' },
              { value: 'weekly', label: '周报' },
              { value: 'monthly', label: '月报' },
            ]}
          />
          <DatePicker value={anchor} onChange={(v) => v && setAnchor(v)} />
          <Button onClick={load}>选择来源</Button>
          <Button type="primary" disabled={!sources.length} loading={loading} onClick={generate}>
            生成草稿
          </Button>
        </Space>
      </Card>
      {!sources.length && (
        <Alert
          style={{ marginTop: 16 }}
          message="请先选择周期并加载来源；无来源时系统不会调用 AI。"
        />
      )}
      <List
        header={`来源笔记（${sources.length}）`}
        dataSource={sources}
        renderItem={(s) => (
          <List.Item>
            <a href={`/notes/${s.id}`}>{s.title}</a>
            <span>{s.note_date}</span>
          </List.Item>
        )}
      />
      {content && (
        <Card title="报告预览">
          <Input value={title} onChange={(e) => setTitle(e.target.value)} />
          <Input.TextArea
            rows={16}
            value={content}
            onChange={(e) => setContent(e.target.value)}
            style={{ marginTop: 12 }}
          />
          <Button style={{ marginTop: 12 }} onClick={save}>
            确认保存
          </Button>
        </Card>
      )}
    </div>
  );
}
