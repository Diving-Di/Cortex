import { useState } from 'react';
import {
  Button,
  Card,
  Form,
  Input,
  InputNumber,
  List,
  Modal,
  Popconfirm,
  Select,
  Space,
  Switch,
  Typography,
  message,
} from 'antd';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  confirmMemoryDraft,
  createMemory,
  createMemoryDraft,
  deleteMemory,
  getMemorySettings,
  listMemories,
  rejectMemoryDraft,
  saveMemorySettings,
  updateMemory,
  type GrowthMemory,
  type MemoryCategory,
  type MemoryDraft,
} from '../../api/memories';
const categories = [
  { value: 'fact', label: '事实' },
  { value: 'preference', label: '偏好' },
  { value: 'goal', label: '目标' },
  { value: 'habit', label: '习惯' },
  { value: 'milestone', label: '里程碑' },
];
export default function MemoriesPage({ token }: { token: string }) {
  const qc = useQueryClient(),
    [search, setSearch] = useState(''),
    [category, setCategory] = useState(''),
    [editing, setEditing] = useState<Partial<GrowthMemory>>(),
    [sourceType, setSourceType] = useState('note'),
    [sourceID, setSourceID] = useState<number>(),
    [draft, setDraft] = useState<MemoryDraft>();
  const memories = useQuery({
    queryKey: ['memories', search, category],
    queryFn: () => listMemories(token, search, category),
  });
  const settings = useQuery({
    queryKey: ['memory-settings'],
    queryFn: () => getMemorySettings(token),
  });
  const save = useMutation({
    mutationFn: (v: Partial<GrowthMemory>) =>
      v.id ? updateMemory(token, v as GrowthMemory) : createMemory(token, v),
    onSuccess: async () => {
      setEditing(undefined);
      await qc.invalidateQueries({ queryKey: ['memories'] });
    },
  });
  const remove = useMutation({
    mutationFn: (id: number) => deleteMemory(token, id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['memories'] }),
  });
  return (
    <div>
      <Typography.Title level={2}>成长记忆</Typography.Title>
      <Card>
        <Space wrap>
          <Input.Search placeholder="搜索记忆" onSearch={setSearch} />
          <Select
            allowClear
            placeholder="全部分类"
            options={categories}
            onChange={(v) => setCategory(v || '')}
          />
          <Button type="primary" onClick={() => setEditing({ category: 'fact', importance: 5 })}>
            新增记忆
          </Button>
        </Space>
      </Card>
      <Card title="AI 记忆建议（确认后才保存）" style={{ marginTop: 16 }}>
        <Space wrap>
          <Select
            value={sourceType}
            onChange={setSourceType}
            options={[
              { value: 'note', label: '笔记 ID' },
              { value: 'conversation', label: '会话 ID' },
              { value: 'message', label: '消息 ID' },
            ]}
          />
          <InputNumber min={1} value={sourceID} onChange={(v) => setSourceID(v || undefined)} />
          <Button
            disabled={!settings.data?.suggestion_enabled || !sourceID}
            onClick={async () => setDraft(await createMemoryDraft(token, sourceType, sourceID!))}
          >
            生成建议
          </Button>
        </Space>
      </Card>
      <Card title="记忆提取策略" style={{ marginTop: 16 }}>
        {settings.data && (
          <Space wrap>
            <Switch
              checked={settings.data.suggestion_enabled}
              onChange={async (v) => {
                await saveMemorySettings(token, { ...settings.data!, suggestion_enabled: v });
                await qc.invalidateQueries({ queryKey: ['memory-settings'] });
                message.success('设置已保存');
              }}
            />
            <span>生成 AI 记忆建议（仍需确认）</span>
            <span>最低重要度</span>
            <InputNumber
              min={1}
              max={10}
              value={settings.data.minimum_importance}
              onChange={async (v) => {
                await saveMemorySettings(token, { ...settings.data!, minimum_importance: v || 5 });
                await qc.invalidateQueries({ queryKey: ['memory-settings'] });
              }}
            />
          </Space>
        )}
      </Card>
      <Card style={{ marginTop: 16 }}>
        <List
          loading={memories.isLoading}
          dataSource={memories.data?.items || []}
          renderItem={(m) => (
            <List.Item
              actions={[
                <Button key="edit" onClick={() => setEditing(m)}>
                  编辑
                </Button>,
                <Popconfirm key="del" title="删除成长记忆？" onConfirm={() => remove.mutate(m.id)}>
                  <Button danger>删除</Button>
                </Popconfirm>,
              ]}
            >
              <List.Item.Meta
                title={`${categories.find((c) => c.value === m.category)?.label} · 重要度 ${m.importance}`}
                description={m.content}
              />
            </List.Item>
          )}
        />
      </Card>
      <Modal
        open={!!editing}
        title={editing?.id ? '编辑记忆' : '新增记忆'}
        footer={null}
        onCancel={() => setEditing(undefined)}
        destroyOnClose
      >
        <Form
          initialValues={editing}
          layout="vertical"
          onFinish={(v) => save.mutate({ ...editing, ...v })}
        >
          <Form.Item name="category" label="分类" rules={[{ required: true }]}>
            <Select options={categories} />
          </Form.Item>
          <Form.Item name="content" label="内容" rules={[{ required: true }, { max: 5000 }]}>
            <Input.TextArea rows={5} />
          </Form.Item>
          <Form.Item name="importance" label="重要度">
            <InputNumber min={1} max={10} />
          </Form.Item>
          <Button htmlType="submit" type="primary" loading={save.isPending}>
            保存
          </Button>
        </Form>
      </Modal>
      <Modal
        open={!!draft}
        title="确认成长记忆建议"
        onCancel={async () => {
          if (draft) await rejectMemoryDraft(token, draft.draft_id);
          setDraft(undefined);
        }}
        onOk={async () => {
          if (draft) {
            await confirmMemoryDraft(token, draft);
            setDraft(undefined);
            await qc.invalidateQueries({ queryKey: ['memories'] });
          }
        }}
      >
        <List
          dataSource={draft?.items || []}
          locale={{ emptyText: '没有符合策略的记忆建议' }}
          renderItem={(item) => (
            <List.Item>
              <List.Item.Meta
                title={`${categories.find((c) => c.value === item.category)?.label} · 重要度 ${item.importance}`}
                description={item.content}
              />
            </List.Item>
          )}
        />
      </Modal>
    </div>
  );
}
