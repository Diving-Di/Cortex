import { useEffect, useRef, useState } from 'react';
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
  listMyTemplates,
  listPublicTemplates,
  publishTemplate,
  recordTemplateView,
  reportTemplate,
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
    [editing, setEditing] = useState<any>(null),
    [reporting, setReporting] = useState<string | null>(null),
    [reportReason, setReportReason] = useState('inappropriate'),
    [reportDetails, setReportDetails] = useState(''),
    viewed = useRef(new Set<string>());
  const pub = useInfiniteQuery({
    queryKey: ['templates', 'public', ranking],
    queryFn: ({ pageParam }) => listPublicTemplates(ranking, pageParam),
    initialPageParam: '',
    getNextPageParam: (last) => last.next_cursor || undefined,
  });
  const mine = useQuery({ queryKey: ['templates', 'mine'], queryFn: () => listMyTemplates() });
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
    mutationFn: async (v: { type: string; id?: number; publicId?: string }) => {
      if (v.type === 'publish') return publishTemplate(v.id!);
      if (v.type === 'withdraw') return withdrawTemplate(v.id!);
      if (v.type === 'delete') return deleteTemplate(v.id!);
      if (v.type === 'use-private') {
        const x = await usePrivateTemplate(v.id!);
        nav(`/notes/${x.note_id}`);
        return;
      }
      const x = await useTemplate(v.publicId!);
      nav(`/notes/${x.note_id}`);
    },
    onSuccess: refresh,
    onError: (e: any) => message.error(e?.response?.data?.message || '操作失败'),
  });
  return (
    <div>
      <Space style={{ marginBottom: 16 }}>
        <Button
          onClick={() => {
            localStorage.setItem('diary:notes-section', 'list');
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
                <Select
                  aria-label="榜单"
                  style={{ width: 160, marginBottom: 16 }}
                  value={ranking}
                  onChange={setRanking}
                  options={[
                    { value: 'recommended', label: '为你推荐' },
                    { value: 'daily', label: '今日热门' },
                    { value: 'trending', label: '近期趋势' },
                    { value: 'new', label: '最新上架' },
                  ]}
                />
                <Row gutter={[16, 16]}>
                  {pub.data?.pages
                    .flatMap((page) => page.items)
                    .map((x) => (
                      <Col xs={24} lg={12} key={x.public_id}>
                        <Card title={x.title} extra={<Tag>{x.category}</Tag>}>
                          <p>{x.description}</p>
                          <SafeMarkdown>{x.content_markdown}</SafeMarkdown>
                          <Space>
                            <Button
                              type="primary"
                              onClick={() => action.mutate({ type: 'use', publicId: x.public_id })}
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
                            <Button onClick={() => setReporting(x.public_id)}>举报</Button>
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
                        <Button onClick={() => action.mutate({ type: 'use-private', id: x.id })}>
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
      <Modal
        open={!!reporting}
        title="举报模板"
        okText="提交"
        onCancel={() => setReporting(null)}
        onOk={async () => {
          if (!reporting) return;
          try {
            await reportTemplate(reporting, reportReason, reportDetails);
            message.success('举报已提交');
            setReporting(null);
            setReportDetails('');
          } catch (error: any) {
            message.error(error?.response?.data?.message || '举报失败');
          }
        }}
      >
        <Select
          aria-label="举报原因"
          value={reportReason}
          onChange={setReportReason}
          style={{ width: '100%', marginBottom: 12 }}
          options={[
            { value: 'inappropriate', label: '不适当内容' },
            { value: 'spam', label: '垃圾内容' },
            { value: 'copyright', label: '版权问题' },
            { value: 'privacy', label: '隐私问题' },
            { value: 'other', label: '其他' },
          ]}
        />
        <Input.TextArea
          aria-label="举报说明"
          rows={4}
          maxLength={1000}
          value={reportDetails}
          onChange={(event) => setReportDetails(event.target.value)}
        />
      </Modal>
      {pub.error && <Alert type="error" message="模板广场加载失败" />}
    </div>
  );
}
