import { useEffect, useMemo, useState } from 'react';
import {
  Alert,
  Button,
  Card,
  Descriptions,
  Drawer,
  Empty,
  Form,
  Image,
  Input,
  InputNumber,
  Modal,
  Progress,
  Popconfirm,
  Radio,
  Select,
  Space,
  Spin,
  Table,
  Tabs,
  Tag,
  Typography,
  message,
} from 'antd';
import {
  CloseCircleOutlined,
  EyeOutlined,
  PlusOutlined,
  ReloadOutlined,
  SaveOutlined,
} from '@ant-design/icons';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import type { ColumnsType } from 'antd/es/table';
import type { FormInstance } from 'antd';
import { listKnowledgeCollections } from '../../api/knowledge';
import {
  batchIgnoreResearchSources,
  batchSaveResearchSources,
  cancelResearchJob,
  createResearchJob,
  deleteResearchSource,
  getResearchSource,
  ignoreResearchSource,
  listResearchJobs,
  listResearchSources,
  loadResearchAsset,
  retryResearchJob,
  recollectResearchSource,
  saveResearchSource,
  updateResearchDraft,
  getXHSAuthorization,
  startXHSAuthorization,
  getXHSAuthAttempt,
  loadXHSAuthQR,
  cancelXHSAuthorization,
  verifyXHSAuthorization,
  revokeXHSAuthorization,
} from '../../api/research';
import type { ResearchAsset, ResearchDraft, ResearchJob, ResearchSource } from '../../api/research';
import './ResearchPage.css';

interface Props {
  token: string;
}

const jobStatus: Record<ResearchJob['status'], { label: string; color: string }> = {
  queued: { label: '等待处理', color: 'default' },
  collecting: { label: '正在采集', color: 'processing' },
  extracting: { label: '正在解析', color: 'processing' },
  organizing: { label: 'AI 整理中', color: 'processing' },
  reviewing: { label: '等待审核', color: 'warning' },
  completed: { label: '已完成', color: 'success' },
  failed: { label: '处理失败', color: 'error' },
  cancelled: { label: '已取消', color: 'default' },
};

const sourceStatus: Record<ResearchSource['status'], { label: string; color: string }> = {
  pending: { label: '等待处理', color: 'default' },
  collecting: { label: '正在采集', color: 'processing' },
  organizing: { label: '正在整理', color: 'processing' },
  pending_review: { label: '待确认', color: 'warning' },
  saved: { label: '已保存', color: 'success' },
  ignored: { label: '已忽略', color: 'default' },
  failed: { label: '失败', color: 'error' },
};

const unknownStatus = { label: '未知状态', color: 'default' };

type CreateValues = {
  mode: 'keyword' | 'urls';
  input: string;
  target_count: number;
  target_collection_id?: number;
};

export default function ResearchPage({ token }: Props) {
  const queryClient = useQueryClient();
  const [createOpen, setCreateOpen] = useState(false);
  const [form] = Form.useForm<CreateValues>();
  const mode = Form.useWatch('mode', form) || 'keyword';
  const [jobPage, setJobPage] = useState(1);
  const [sourcePage, setSourcePage] = useState(1);
  const [sourceStatusFilter, setSourceStatusFilter] = useState<string>();
  const [sourceSearch, setSourceSearch] = useState('');
  const [sourceSort, setSourceSort] = useState('created_at');
  const [selectedIDs, setSelectedIDs] = useState<number[]>([]);
  const [selectedSourceID, setSelectedSourceID] = useState<number>();
  const [draftForm] = Form.useForm<Record<string, string>>();
  const [authAttemptID, setAuthAttemptID] = useState<string>();
  const [authQR, setAuthQR] = useState<string>();
  const [authQRError, setAuthQRError] = useState(false);

  const authorization = useQuery({
    queryKey: ['xhs-authorization'],
    queryFn: () => getXHSAuthorization(token),
    retry: false,
  });
  const coreEnabled = authorization.data?.status === 'authorized';
  const authAttempt = useQuery({
    queryKey: ['xhs-auth-attempt', authAttemptID],
    queryFn: () => getXHSAuthAttempt(token, authAttemptID!),
    enabled: Boolean(authAttemptID),
    refetchInterval: (query) =>
      query.state.data &&
      ['authorized', 'failed', 'cancelled', 'expired'].includes(query.state.data.status)
        ? false
        : 2000,
  });

  useEffect(() => {
    const status = authAttempt.data?.status;
    if (
      !authAttemptID ||
      !status ||
      !['waiting_for_scan', 'verification_required'].includes(status)
    ) {
      return;
    }
    let active = true;
    setAuthQRError(false);
    loadXHSAuthQR(token, authAttemptID)
      .then((url) => {
        if (!active) return URL.revokeObjectURL(url);
        setAuthQR((previous) => {
          if (previous) URL.revokeObjectURL(previous);
          return url;
        });
      })
      .catch(() => {
        if (active) setAuthQRError(true);
      });
    return () => {
      active = false;
    };
  }, [authAttempt.dataUpdatedAt, authAttempt.data?.status, authAttemptID, token]);

  useEffect(() => {
    if (authAttempt.data?.status === 'authorized') {
      void queryClient.invalidateQueries({ queryKey: ['xhs-authorization'] });
      message.success('小红书授权成功');
      setAuthAttemptID(undefined);
    }
  }, [authAttempt.data?.status, queryClient]);

  useEffect(
    () => () => {
      if (authQR) URL.revokeObjectURL(authQR);
    },
    [authQR],
  );

  const startAuthorization = useMutation({
    mutationFn: () => startXHSAuthorization(token),
    onSuccess: (attempt) => {
      setAuthQR(undefined);
      setAuthQRError(false);
      setAuthAttemptID(attempt.id);
    },
  });
  const verifyAuthorization = useMutation({
    mutationFn: () => verifyXHSAuthorization(token),
    onSuccess: async () => {
      message.success('授权有效');
      await queryClient.invalidateQueries({ queryKey: ['xhs-authorization'] });
    },
  });
  const revokeAuthorization = useMutation({
    mutationFn: () => revokeXHSAuthorization(token),
    onSuccess: async () => {
      message.success('授权已撤销');
      await queryClient.invalidateQueries({ queryKey: ['xhs-authorization'] });
    },
  });

  const jobs = useQuery({
    queryKey: ['research-jobs', jobPage],
    queryFn: () => listResearchJobs(token, jobPage),
    enabled: coreEnabled,
    refetchInterval: (query) =>
      query.state.data?.items.some((item) =>
        ['queued', 'collecting', 'extracting', 'organizing'].includes(item.status),
      )
        ? 3000
        : false,
  });
  const sources = useQuery({
    queryKey: ['research-sources', sourceStatusFilter, sourceSearch, sourceSort, sourcePage],
    queryFn: () =>
      listResearchSources(token, {
        status: sourceStatusFilter,
        search: sourceSearch,
        sort: sourceSort,
        page: sourcePage,
      }),
    enabled: coreEnabled,
  });
  const selectedSource = useQuery({
    queryKey: ['research-source', selectedSourceID],
    queryFn: () => getResearchSource(token, selectedSourceID!),
    enabled: coreEnabled && Boolean(selectedSourceID),
  });
  const collections = useQuery({
    queryKey: ['knowledge-collections'],
    queryFn: () => listKnowledgeCollections(token),
    enabled: coreEnabled,
  });

  useEffect(() => {
    if (selectedSource.data?.draft) {
      draftForm.setFieldsValue({
        summary: selectedSource.data.draft.summary,
        category: selectedSource.data.draft.category,
        key_points: selectedSource.data.draft.key_points.join('\n'),
        suggested_tags: selectedSource.data.draft.suggested_tags.join(', '),
      });
    }
  }, [draftForm, selectedSource.data]);

  const refresh = async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ['research-jobs'] }),
      queryClient.invalidateQueries({ queryKey: ['research-sources'] }),
      queryClient.invalidateQueries({ queryKey: ['research-source'] }),
    ]);
  };

  const create = useMutation({
    mutationFn: (values: CreateValues) => {
      const lines = values.input
        .split(/\r?\n/)
        .map((item) => item.trim())
        .filter(Boolean);
      return createResearchJob(token, {
        mode: values.mode,
        ...(values.mode === 'keyword' ? { keywords: lines } : { urls: lines }),
        target_count: values.target_count,
        target_collection_id: values.target_collection_id,
        idempotency_key: crypto.randomUUID(),
      });
    },
    onSuccess: async () => {
      setCreateOpen(false);
      form.resetFields();
      message.success('研究任务已加入队列');
      await refresh();
    },
  });
  const cancel = useMutation({
    mutationFn: (id: number) => cancelResearchJob(token, id),
    onSuccess: refresh,
  });
  const retry = useMutation({
    mutationFn: (id: number) => retryResearchJob(token, id),
    onSuccess: refresh,
  });
  const save = useMutation({
    mutationFn: (id: number) => saveResearchSource(token, id),
    onSuccess: async () => {
      message.success('已保存到个人知识库');
      await refresh();
    },
  });
  const ignore = useMutation({
    mutationFn: (id: number) => ignoreResearchSource(token, id),
    onSuccess: refresh,
  });
  const batchSave = useMutation({
    mutationFn: (ids: number[]) => batchSaveResearchSources(token, ids),
    onSuccess: async () => {
      setSelectedIDs([]);
      message.success('所选结果已保存');
      await refresh();
    },
  });
  const batchIgnore = useMutation({
    mutationFn: (ids: number[]) => batchIgnoreResearchSources(token, ids),
    onSuccess: async () => {
      setSelectedIDs([]);
      await refresh();
    },
  });
  const updateDraft = useMutation({
    mutationFn: (value: {
      source: ResearchSource;
      draft: ResearchDraft;
      fields: Record<string, string>;
    }) =>
      updateResearchDraft(token, value.source.id, {
        summary: value.fields.summary,
        category: value.fields.category,
        key_points: splitLines(value.fields.key_points),
        suggested_tags: value.fields.suggested_tags
          .split(',')
          .map((item) => item.trim())
          .filter(Boolean),
        version: value.draft.version,
      }),
    onSuccess: async () => {
      message.success('草稿已更新');
      await refresh();
    },
  });
  const recollect = useMutation({
    mutationFn: (id: number) => recollectResearchSource(token, id),
    onSuccess: async () => {
      message.success('已创建重新采集任务');
      await refresh();
    },
  });
  const remove = useMutation({
    mutationFn: (id: number) => deleteResearchSource(token, id),
    onSuccess: async () => {
      setSelectedSourceID(undefined);
      message.success('研究来源已删除，并停止参与知识检索');
      await refresh();
    },
  });

  const jobColumns: ColumnsType<ResearchJob> = [
    {
      title: '研究内容',
      render: (_, item) =>
        item.mode === 'keyword'
          ? item.query_payload.keywords?.join('、')
          : `${item.query_payload.urls?.length || 0} 个链接`,
    },
    {
      title: '进度',
      width: 210,
      render: (_, item) => {
        const done = item.organized_count + item.failed_count;
        return (
          <Progress
            size="small"
            percent={Math.min(100, Math.round((done / Math.max(item.target_count, 1)) * 100))}
            format={() => `${done}/${item.target_count}`}
          />
        );
      },
    },
    {
      title: '状态',
      width: 110,
      render: (_, item) => {
        const status = jobStatus[item.status] || unknownStatus;
        return <Tag color={status.color}>{status.label}</Tag>;
      },
    },
    {
      title: '创建时间',
      width: 170,
      render: (_, item) => new Date(item.created_at).toLocaleString(),
    },
    {
      title: '操作',
      width: 120,
      render: (_, item) =>
        ['failed', 'cancelled'].includes(item.status) ? (
          <Button type="link" icon={<ReloadOutlined />} onClick={() => retry.mutate(item.id)}>
            重试
          </Button>
        ) : !['completed', 'reviewing'].includes(item.status) ? (
          <Button type="link" danger onClick={() => cancel.mutate(item.id)}>
            取消
          </Button>
        ) : null,
    },
  ];

  const sourceColumns: ColumnsType<ResearchSource> = [
    {
      title: '内容',
      render: (_, item) => (
        <div>
          <Typography.Text strong>{item.title || '等待解析'}</Typography.Text>
          <div className="research-source-meta">{item.author_display_name || '公开来源'}</div>
        </div>
      ),
    },
    {
      title: '分类',
      width: 140,
      render: (_, item) => item.draft?.category || '—',
    },
    {
      title: '状态',
      width: 110,
      render: (_, item) => {
        const status = sourceStatus[item.status] || unknownStatus;
        return <Tag color={status.color}>{status.label}</Tag>;
      },
    },
    {
      title: '采集时间',
      width: 170,
      render: (_, item) => new Date(item.collected_at || item.created_at).toLocaleString(),
    },
    {
      title: '操作',
      width: 90,
      render: (_, item) => (
        <Button type="link" icon={<EyeOutlined />} onClick={() => setSelectedSourceID(item.id)}>
          查看
        </Button>
      ),
    },
  ];

  const actionableSelected = useMemo(
    () =>
      (sources.data?.items || [])
        .filter((item) => selectedIDs.includes(item.id) && item.status === 'pending_review')
        .map((item) => item.id),
    [selectedIDs, sources.data],
  );
  const pageError = authorization.error || jobs.error || sources.error || collections.error;

  if (authorization.isLoading) {
    return (
      <div className="research-auth-gate research-auth-loading">
        <Spin size="large" />
        <Typography.Text type="secondary">正在检查小红书授权状态…</Typography.Text>
      </div>
    );
  }

  if (authorization.isError) {
    return (
      <div className="research-auth-gate">
        <Typography.Title level={2}>小红书研究</Typography.Title>
        <Alert
          showIcon
          type="error"
          message="授权状态加载失败"
          description="请检查网络或服务状态后重试。"
          action={<Button onClick={() => void authorization.refetch()}>重试</Button>}
        />
      </div>
    );
  }

  if (!coreEnabled) {
    const attemptFailed =
      authAttempt.data && ['failed', 'cancelled', 'expired'].includes(authAttempt.data.status);
    return (
      <div className="research-auth-gate">
        <div>
          <Typography.Title level={2}>授权小红书账号</Typography.Title>
          <Typography.Text type="secondary">
            完成当前个人空间的扫码授权后，才能进入小红书研究页面。
          </Typography.Text>
        </div>
        <Alert
          showIcon
          type="info"
          message="授权凭据按个人租户隔离加密保存"
          description="凭据仅用于你主动发起的研究任务，不会展示给其他账号。"
        />
        <Card className="research-auth-gate-card">
          <Space direction="vertical" size="large">
            <div>
              <Typography.Title level={4}>开始扫码授权</Typography.Title>
              <Typography.Paragraph type="secondary">
                点击下方按钮生成限时二维码，再使用小红书 App 扫描并确认登录。
              </Typography.Paragraph>
            </div>
            {startAuthorization.isError && !authAttemptID ? (
              <Alert showIcon type="error" message="无法创建授权任务，请稍后重试。" />
            ) : null}
            <Button
              type="primary"
              size="large"
              loading={startAuthorization.isPending}
              onClick={() => {
                startAuthorization.reset();
                startAuthorization.mutate();
              }}
            >
              打开扫码授权窗口
            </Button>
          </Space>
        </Card>

        <Modal
          title="扫码授权小红书"
          open={Boolean(authAttemptID)}
          footer={
            <Button
              onClick={async () => {
                if (authAttemptID) await cancelXHSAuthorization(token, authAttemptID);
                setAuthAttemptID(undefined);
                startAuthorization.reset();
              }}
            >
              取消授权
            </Button>
          }
          onCancel={() => setAuthAttemptID(undefined)}
        >
          <div className="research-auth-modal">
            {attemptFailed ? (
              <Alert
                showIcon
                type="error"
                message="授权任务未完成"
                description={authAttempt.data?.failure_code || '二维码已失效，请关闭窗口后重试。'}
              />
            ) : authQR ? (
              <Image preview={false} src={authQR} alt="小红书登录二维码页面" />
            ) : (
              <Spin size="large" />
            )}
            {authQRError && !attemptFailed ? (
              <Alert showIcon type="warning" message="二维码仍在生成，页面会自动重试。" />
            ) : null}
            {!attemptFailed ? (
              <Typography.Text type="secondary">
                {authAttempt.data?.status === 'verification_required'
                  ? '页面需要安全验证，请稍后重试或重新扫码。'
                  : '请使用小红书 App 扫描页面中的二维码，授权完成后会自动进入研究页面。'}
              </Typography.Text>
            ) : null}
          </div>
        </Modal>
      </div>
    );
  }

  return (
    <div className="research-page">
      <div className="research-header">
        <div>
          <Typography.Title level={2}>小红书研究</Typography.Title>
          <Typography.Text type="secondary">
            收集公开内容，提炼观点并保存到个人知识库。
          </Typography.Text>
        </div>
        <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreateOpen(true)}>
          新建研究
        </Button>
      </div>

      <Alert
        showIcon
        type="info"
        message="仅处理你有权研究或保存的公开内容，请遵守平台规则、版权和适用法律。"
      />

      {pageError ? (
        <Alert
          showIcon
          type="error"
          message="研究页面数据加载失败"
          description="请检查网络或服务状态后重试。"
          action={<Button onClick={() => void refresh()}>重试</Button>}
        />
      ) : null}

      <Card size="small" className="research-auth-card">
        <div>
          <Space>
            <Typography.Text strong>小红书账号授权</Typography.Text>
            <Tag color={authorization.data?.status === 'authorized' ? 'success' : 'default'}>
              {authorization.data?.status === 'authorized' ? '已授权' : '未授权'}
            </Tag>
          </Space>
          <div className="research-source-meta">
            授权按当前个人租户隔离保存，仅用于你发起的研究任务。
          </div>
        </div>
        <Space wrap>
          {authorization.data?.status === 'authorized' && (
            <Button
              loading={verifyAuthorization.isPending}
              onClick={() => verifyAuthorization.mutate()}
            >
              验证授权
            </Button>
          )}
          <Button
            type={authorization.data?.status === 'authorized' ? 'default' : 'primary'}
            onClick={() => startAuthorization.mutate()}
          >
            {authorization.data?.status === 'authorized' ? '重新授权' : '扫码授权'}
          </Button>
          {authorization.data?.status === 'authorized' && (
            <Popconfirm
              title="撤销后，正在运行的研究任务也会取消。确定撤销？"
              onConfirm={() => revokeAuthorization.mutate()}
            >
              <Button danger>撤销</Button>
            </Popconfirm>
          )}
        </Space>
      </Card>

      <Tabs
        items={[
          {
            key: 'jobs',
            label: '研究任务',
            children: (
              <Card>
                <Table
                  rowKey="id"
                  loading={jobs.isLoading}
                  dataSource={jobs.data?.items || []}
                  columns={jobColumns}
                  locale={{ emptyText: <Empty description="尚未创建研究任务" /> }}
                  pagination={{
                    current: jobPage,
                    pageSize: 20,
                    total: jobs.data?.total || 0,
                    showSizeChanger: false,
                    onChange: setJobPage,
                  }}
                />
              </Card>
            ),
          },
          {
            key: 'sources',
            label: '研究结果',
            children: (
              <Card>
                <Space className="research-toolbar" wrap>
                  <Input.Search
                    aria-label="搜索研究结果"
                    allowClear
                    placeholder="搜索标题或作者"
                    onSearch={(value) => {
                      setSourceSearch(value.trim());
                      setSourcePage(1);
                    }}
                  />
                  <Select
                    aria-label="筛选研究状态"
                    allowClear
                    placeholder="全部状态"
                    value={sourceStatusFilter}
                    onChange={(value) => {
                      setSourceStatusFilter(value);
                      setSourcePage(1);
                    }}
                    options={Object.entries(sourceStatus).map(([value, item]) => ({
                      value,
                      label: item.label,
                    }))}
                  />
                  <Select
                    aria-label="研究结果排序"
                    value={sourceSort}
                    onChange={setSourceSort}
                    options={[
                      { value: 'created_at', label: '按采集时间' },
                      { value: 'published_at', label: '按发布时间' },
                    ]}
                  />
                  <Button
                    icon={<SaveOutlined />}
                    disabled={!actionableSelected.length}
                    onClick={() => batchSave.mutate(actionableSelected)}
                  >
                    批量保存
                  </Button>
                  <Button
                    disabled={!actionableSelected.length}
                    onClick={() => batchIgnore.mutate(actionableSelected)}
                  >
                    批量忽略
                  </Button>
                </Space>
                <Table
                  rowKey="id"
                  loading={sources.isLoading}
                  dataSource={sources.data?.items || []}
                  columns={sourceColumns}
                  rowSelection={{
                    selectedRowKeys: selectedIDs,
                    onChange: (keys) => setSelectedIDs(keys as number[]),
                  }}
                  locale={{ emptyText: <Empty description="尚无研究结果" /> }}
                  pagination={{
                    current: sourcePage,
                    pageSize: 20,
                    total: sources.data?.total || 0,
                    showSizeChanger: false,
                    onChange: setSourcePage,
                  }}
                />
              </Card>
            ),
          },
        ]}
      />

      <Modal
        title="扫码授权小红书"
        open={Boolean(authAttemptID)}
        footer={
          <Button
            onClick={async () => {
              if (authAttemptID) await cancelXHSAuthorization(token, authAttemptID);
              setAuthAttemptID(undefined);
            }}
          >
            取消授权
          </Button>
        }
        onCancel={() => setAuthAttemptID(undefined)}
      >
        <div className="research-auth-modal">
          {authQR ? (
            <Image preview={false} src={authQR} alt="小红书登录二维码页面" />
          ) : (
            <Progress type="circle" percent={30} status="active" />
          )}
          <Typography.Text type="secondary">
            {authAttempt.data?.status === 'verification_required'
              ? '页面需要安全验证，请稍后重试或重新扫码。'
              : '请使用小红书 App 扫描页面中的二维码，授权完成后会自动关闭。'}
          </Typography.Text>
        </div>
      </Modal>

      <Modal
        title="新建研究"
        open={createOpen}
        onCancel={() => setCreateOpen(false)}
        onOk={() => form.submit()}
        confirmLoading={create.isPending}
        okText="开始研究"
      >
        <Form<CreateValues>
          form={form}
          layout="vertical"
          initialValues={{ mode: 'keyword', target_count: 10 }}
          onFinish={(values) => create.mutate(values)}
        >
          <Form.Item name="mode" label="研究方式">
            <Radio.Group>
              <Radio.Button value="keyword">关键词搜索</Radio.Button>
              <Radio.Button value="urls">笔记链接</Radio.Button>
            </Radio.Group>
          </Form.Item>
          <Form.Item
            name="input"
            label={mode === 'keyword' ? '研究关键词' : '公开笔记链接'}
            extra="每行填写一项"
            rules={[{ required: true, message: '请输入研究内容' }]}
          >
            <Input.TextArea
              rows={6}
              placeholder={
                mode === 'keyword'
                  ? 'Agent 面试\nRAG 实践'
                  : 'https://www.xiaohongshu.com/explore/...'
              }
            />
          </Form.Item>
          <Form.Item name="target_count" label="目标结果数" rules={[{ required: true }]}>
            <InputNumber min={1} max={50} />
          </Form.Item>
          <Form.Item name="target_collection_id" label="目标知识集合（可选）">
            <Select
              allowClear
              options={(collections.data || []).map((item) => ({
                value: item.id,
                label: item.name,
              }))}
            />
          </Form.Item>
        </Form>
      </Modal>

      <Drawer
        width={720}
        title="研究详情"
        open={Boolean(selectedSourceID)}
        onClose={() => setSelectedSourceID(undefined)}
        loading={selectedSource.isLoading}
        extra={
          selectedSource.data ? (
            <Space wrap>
              <Popconfirm
                title="删除研究来源？"
                description="关联的知识文档和图片也会停止使用。"
                onConfirm={() => remove.mutate(selectedSource.data!.id)}
              >
                <Button danger>删除</Button>
              </Popconfirm>
              {selectedSource.data.status === 'pending_review' ? (
                <>
                  <Button
                    icon={<CloseCircleOutlined />}
                    onClick={() => ignore.mutate(selectedSource.data!.id)}
                  >
                    忽略
                  </Button>
                  <Button
                    type="primary"
                    icon={<SaveOutlined />}
                    onClick={() => save.mutate(selectedSource.data!.id)}
                  >
                    保存到知识库
                  </Button>
                </>
              ) : selectedSource.data.status === 'failed' ? (
                <Button
                  icon={<ReloadOutlined />}
                  onClick={() => recollect.mutate(selectedSource.data!.id)}
                >
                  重新采集
                </Button>
              ) : null}
            </Space>
          ) : null
        }
      >
        {selectedSource.data ? (
          <ResearchDetail
            token={token}
            source={selectedSource.data}
            form={draftForm}
            onSaveDraft={(fields) => {
              if (selectedSource.data?.draft) {
                updateDraft.mutate({
                  source: selectedSource.data,
                  draft: selectedSource.data.draft,
                  fields,
                });
              }
            }}
          />
        ) : null}
      </Drawer>
    </div>
  );
}

function ResearchDetail({
  token,
  source,
  form,
  onSaveDraft,
}: {
  token: string;
  source: ResearchSource;
  form: FormInstance<Record<string, string>>;
  onSaveDraft: (value: Record<string, string>) => void;
}) {
  return (
    <Space direction="vertical" size="large" className="research-detail">
      <Descriptions column={1} size="small">
        <Descriptions.Item label="标题">{source.title}</Descriptions.Item>
        <Descriptions.Item label="作者">{source.author_display_name || '—'}</Descriptions.Item>
        <Descriptions.Item label="来源">
          <a href={source.source_url} target="_blank" rel="noreferrer">
            打开公开来源
          </a>
        </Descriptions.Item>
      </Descriptions>
      {source.failure_summary ? (
        <Alert type="error" showIcon message={source.failure_summary} />
      ) : null}
      <section>
        <Typography.Title level={4}>来源原文</Typography.Title>
        <div className="research-content">{source.raw_content || '暂无正文'}</div>
      </section>
      {source.assets?.length ? (
        <section>
          <Typography.Title level={4}>来源图片与 OCR</Typography.Title>
          <div className="research-assets">
            {source.assets.map((asset) => (
              <ResearchAssetView key={asset.id} token={token} asset={asset} />
            ))}
          </div>
        </section>
      ) : null}
      {source.draft ? (
        <section>
          <Typography.Title level={4}>AI 整理草稿</Typography.Title>
          <Form form={form} layout="vertical" onFinish={onSaveDraft}>
            <Form.Item name="summary" label="摘要" rules={[{ required: true }]}>
              <Input.TextArea rows={5} />
            </Form.Item>
            <Form.Item name="key_points" label="关键观点（每行一项）">
              <Input.TextArea rows={5} />
            </Form.Item>
            <Form.Item name="category" label="分类">
              <Input maxLength={120} />
            </Form.Item>
            <Form.Item name="suggested_tags" label="建议标签（逗号分隔）">
              <Input />
            </Form.Item>
            {source.draft.status === 'pending' ? <Button htmlType="submit">更新草稿</Button> : null}
          </Form>
        </section>
      ) : null}
    </Space>
  );
}

function ResearchAssetView({ token, asset }: { token: string; asset: ResearchAsset }) {
  const [url, setURL] = useState<string>();
  useEffect(() => {
    let active = true;
    loadResearchAsset(token, asset.id).then((value) => {
      if (active) setURL(value);
      else URL.revokeObjectURL(value);
    });
    return () => {
      active = false;
      if (url) URL.revokeObjectURL(url);
    };
  }, [asset.id, token]);
  return (
    <Card size="small">
      {url ? <Image src={url} alt={`来源图片 ${asset.position + 1}`} width={180} /> : null}
      <Typography.Paragraph type="secondary">
        OCR：{asset.ocr_status === 'ready' ? asset.ocr_text || '未识别到文字' : '不可用或处理失败'}
      </Typography.Paragraph>
    </Card>
  );
}

function splitLines(value = '') {
  return value
    .split(/\r?\n/)
    .map((item) => item.trim())
    .filter(Boolean);
}
