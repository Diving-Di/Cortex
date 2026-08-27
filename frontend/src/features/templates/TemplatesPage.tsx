import { useEffect, useMemo, useRef, useState } from 'react';
import {
  Alert,
  Button,
  Card,
  Col,
  Empty,
  Form,
  Input,
  List,
  Modal,
  Row,
  Select,
  Space,
  Tabs,
  Tag,
  message,
} from 'antd';
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useNavigate } from 'react-router-dom';
import SafeMarkdown from '../../components/SafeMarkdown';
import {
  createTemplate,
  deleteTemplate,
  getPublicTemplate,
  listMyTemplates,
  listPublicTemplates,
  publishTemplate,
  recordTemplateView,
  savePublicProfile,
  setTemplateReaction,
  updateTemplate,
  usePrivateTemplate,
  useTemplate,
  withdrawTemplate,
} from '../../api/templates';

export default function TemplatesPage() {
  const qc = useQueryClient(),
    nav = useNavigate(),
    [open, setOpen] = useState(false),
    [nickname, setNickname] = useState(''),
    [ranking, setRanking] = useState('recommended'),
    [searchInput, setSearchInput] = useState(''),
    [query, setQuery] = useState(''),
    [category, setCategory] = useState(''),
    [detailId, setDetailId] = useState<string | null>(null),
    [editing, setEditing] = useState<any>(null),
    viewed = useRef(new Set<string>()),
    useIntent = useRef<{ target: string; key: string; inFlight: boolean } | null>(null);
  const pub = useInfiniteQuery({
    queryKey: ['templates', 'public', ranking, query, category],
    queryFn: ({ pageParam }) => listPublicTemplates(ranking, pageParam, query, category),
    initialPageParam: '',
    getNextPageParam: (last) => last.next_cursor || undefined,
  });
  const detail = useQuery({
    queryKey: ['templates', 'public', 'detail', detailId],
    queryFn: () => getPublicTemplate(detailId!),
    enabled: !!detailId,
  });
  const mine = useQuery({ queryKey: ['templates', 'mine'], queryFn: () => listMyTemplates() });
  const categories = useMemo(
    () =>
      Array.from(
        new Set(
          (pub.data?.pages.flatMap((page) => page.items) || [])
            .map((item) => item.category)
            .filter(Boolean),
        ),
      ).sort(),
    [pub.data?.pages],
  );
  useEffect(() => {
    for (const item of pub.data?.pages.flatMap((page) => page.items) || []) {
      if (!viewed.current.has(item.public_id)) {
        viewed.current.add(item.public_id);
        void recordTemplateView(item.public_id);
      }
    }
  }, [pub.data?.pages]);
  const refresh = () => qc.invalidateQueries({ queryKey: ['templates'] });
  const action = useMutation({
    mutationFn: async (v: {
      type: string;
      id?: number;
      publicId?: string;
      idempotencyKey?: string;
    }) => {
      if (v.type === 'publish') return publishTemplate(v.id!);
      if (v.type === 'withdraw') return withdrawTemplate(v.id!);
      if (v.type === 'delete') return deleteTemplate(v.id!);
      if (v.type === 'use-private') {
        const x = await usePrivateTemplate(v.id!, v.idempotencyKey!);
        nav(`/notes/${x.note_id}`);
        return;
      }
      const x = await useTemplate(v.publicId!, v.idempotencyKey!);
      nav(`/notes/${x.note_id}`);
    },
    onSuccess: (_, variables) => {
      if (variables.type === 'use' || variables.type === 'use-private') useIntent.current = null;
      refresh();
    },
    onError: (e: any) => message.error(e?.response?.data?.message || '操作失败'),
    onSettled: (_, __, variables) => {
      const intent = useIntent.current;
      if (
        (variables.type === 'use' || variables.type === 'use-private') &&
        intent &&
        intent.key === variables.idempotencyKey
      ) {
        intent.inFlight = false;
      }
    },
  });
  const createNoteFromTemplate = (value: { id?: number; publicId?: string }) => {
    const target = value.publicId ? `public:${value.publicId}` : `private:${value.id}`;
    if (useIntent.current?.inFlight) return;
    if (useIntent.current?.target !== target) {
      useIntent.current = { target, key: crypto.randomUUID(), inFlight: false };
    }
    const intent = useIntent.current;
    intent.inFlight = true;
    action.mutate({
      type: value.publicId ? 'use' : 'use-private',
      ...value,
      idempotencyKey: intent.key,
    });
  };
  return (
    <div>
      <Space style={{ marginBottom: 16 }}>
        <Button
          onClick={() => {
            localStorage.setItem('cortex:notes-section', 'list');
            nav('/notes/list');
          }}
        >
          我的笔记
        </Button>
      </Space>
      <Tabs
        defaultActiveKey="public"
        items={[
          {
            key: 'public',
            label: '模板广场',
            children: (
              <>
                <Space wrap style={{ marginBottom: 16 }}>
                  <Input.Search
                    aria-label="搜索模板"
                    allowClear
                    placeholder="搜索标题或说明"
                    value={searchInput}
                    onChange={(event) => setSearchInput(event.target.value)}
                    onSearch={(value) => setQuery(value.trim())}
                    style={{ width: 280 }}
                  />
                  <Select
                    aria-label="分类"
                    allowClear
                    showSearch
                    placeholder="全部分类"
                    value={category || undefined}
                    onChange={(value) => setCategory(value || '')}
                    options={categories.map((value) => ({ value, label: value }))}
                    style={{ width: 160 }}
                  />
                  <Select
                    aria-label="榜单"
                    style={{ width: 160 }}
                    value={ranking}
                    onChange={setRanking}
                    options={[
                      { value: 'recommended', label: '为你推荐' },
                      { value: 'daily', label: '今日热门' },
                      { value: 'trending', label: '近期趋势' },
                      { value: 'new', label: '最新上架' },
                    ]}
                  />
                </Space>
                <Row gutter={[16, 16]}>
                  {pub.data?.pages
                    .flatMap((page) => page.items)
                    .map((x) => (
                      <Col xs={24} lg={12} key={x.public_id}>
                        <Card title={x.title} extra={<Tag>{x.category}</Tag>}>
                          <p>{x.description}</p>
                          <Space>
                            <Button onClick={() => setDetailId(x.public_id)}>查看详情</Button>
                            <Button
                              type="primary"
                              disabled={action.isPending}
                              loading={
                                action.isPending && action.variables?.publicId === x.public_id
                              }
                              onClick={() => createNoteFromTemplate({ publicId: x.public_id })}
                            >
                              使用模板
                            </Button>
                            <Button
                              onClick={async () => {
                                await setTemplateReaction(x.public_id, 'like', !x.liked);
                                refresh();
                              }}
                            >
                              {x.liked ? '取消点赞' : '点赞'} {x.like_count}
                            </Button>
                            <Button
                              onClick={async () => {
                                await setTemplateReaction(x.public_id, 'favorite', !x.favorited);
                                refresh();
                              }}
                            >
                              {x.favorited ? '取消收藏' : '收藏'} {x.favorite_count}
                            </Button>
                            <span>
                              {x.author_nickname} · 使用 {x.usage_count}
                            </span>
                          </Space>
                        </Card>
                      </Col>
                    ))}
                  {!pub.isLoading && !pub.data?.pages.some((page) => page.items.length) && (
                    <Empty />
                  )}
                </Row>
                {pub.hasNextPage && (
                  <Button loading={pub.isFetchingNextPage} onClick={() => pub.fetchNextPage()}>
                    加载更多
                  </Button>
                )}
              </>
            ),
          },
          {
            key: 'mine',
            label: '我的模板',
            children: (
              <>
                <Space style={{ marginBottom: 16 }}>
                  <Button type="primary" onClick={() => setOpen(true)}>
                    新建模板
                  </Button>
                  <Input
                    placeholder="公开昵称"
                    value={nickname}
                    onChange={(e) => setNickname(e.target.value)}
                  />
                  <Button
                    onClick={async () => {
                      await savePublicProfile(nickname);
                      message.success('公开昵称已保存');
                    }}
                  >
                    保存昵称
                  </Button>
                </Space>
                <List
                  dataSource={mine.data ?? []}
                  renderItem={(x) => (
                    <List.Item
                      actions={[
                        <Button
                          disabled={action.isPending}
                          loading={action.isPending && action.variables?.id === x.id}
                          onClick={() => createNoteFromTemplate({ id: x.id })}
                        >
                          使用
                        </Button>,
                        <Button onClick={() => setEditing(x)}>编辑</Button>,
                        x.status === 'published' ? (
                          <Space>
                            <Button onClick={() => action.mutate({ type: 'publish', id: x.id })}>
                              更新上架
                            </Button>
                            <Button onClick={() => action.mutate({ type: 'withdraw', id: x.id })}>
                              下架
                            </Button>
                          </Space>
                        ) : (
                          <Button onClick={() => action.mutate({ type: 'publish', id: x.id })}>
                            上架
                          </Button>
                        ),
                        <Button danger onClick={() => action.mutate({ type: 'delete', id: x.id })}>
                          删除
                        </Button>,
                      ]}
                    >
                      <List.Item.Meta title={x.title} description={`${x.category} · ${x.status}`} />
                    </List.Item>
                  )}
                />
              </>
            ),
          },
        ]}
      />
      <Modal
        open={!!detailId}
        title={detail.data?.title || '模板详情'}
        footer={null}
        width={760}
        onCancel={() => setDetailId(null)}
      >
        {detail.isLoading && <div>详情加载中...</div>}
        {detail.isError && <Alert type="error" message="模板详情加载失败" />}
        {detail.data && (
          <>
            <Space wrap style={{ marginBottom: 12 }}>
              <Tag>{detail.data.category}</Tag>
              <span>{detail.data.author_nickname}</span>
              <span>使用 {detail.data.usage_count}</span>
            </Space>
            {detail.data.description && <p>{detail.data.description}</p>}
            <SafeMarkdown>{detail.data.content_markdown}</SafeMarkdown>
          </>
        )}
      </Modal>
      <Modal open={open} footer={null} onCancel={() => setOpen(false)} title="新建模板">
        <Form
          layout="vertical"
          onFinish={async (v) => {
            await createTemplate(v);
            setOpen(false);
            refresh();
          }}
        >
          <Form.Item name="title" label="标题" rules={[{ required: true }]}>
            <Input />
          </Form.Item>
          <Form.Item name="description" label="说明">
            <Input />
          </Form.Item>
          <Form.Item name="category" label="分类" rules={[{ required: true }]}>
            <Input />
          </Form.Item>
          <Form.Item name="content_markdown" label="Markdown" rules={[{ required: true }]}>
            <Input.TextArea rows={10} />
          </Form.Item>
          <Button htmlType="submit" type="primary">
            保存
          </Button>
        </Form>
      </Modal>
      <Modal
        open={!!editing}
        title="编辑模板"
        okText="保存"
        onCancel={() => setEditing(null)}
        onOk={async () => {
          try {
            await updateTemplate(editing);
            setEditing(null);
            refresh();
          } catch (error: any) {
            message.error(error?.response?.data?.message || '模板保存失败');
          }
        }}
      >
        <Input
          value={editing?.title}
          onChange={(e) => setEditing({ ...editing, title: e.target.value })}
        />
        <Input
          value={editing?.category}
          onChange={(e) => setEditing({ ...editing, category: e.target.value })}
        />
        <Input
          value={editing?.description}
          placeholder="说明"
          onChange={(e) => setEditing({ ...editing, description: e.target.value })}
        />
        <Input.TextArea
          rows={10}
          value={editing?.content_markdown}
          onChange={(e) => setEditing({ ...editing, content_markdown: e.target.value })}
        />
        {editing?.content_markdown && <SafeMarkdown>{editing.content_markdown}</SafeMarkdown>}
      </Modal>
      {pub.error && <Alert type="error" message="模板广场加载失败" />}
    </div>
  );
}
