import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  Alert,
  Button,
  Card,
  Empty,
  Popconfirm,
  Progress,
  Space,
  Table,
  Tag,
  Typography,
  Upload,
  message,
} from 'antd';
import { DeleteOutlined, InboxOutlined } from '@ant-design/icons';
import { deleteKnowledge, listKnowledge, uploadKnowledge } from '../../api/knowledge';
import './KnowledgePage.css';

const statusText: Record<string, string> = {
  uploaded: '已上传',
  parsing: '解析中',
  indexing: '索引中',
  ready: '可用',
  failed: '失败',
  deleting: '删除中',
};
export default function KnowledgePage() {
  const queryClient = useQueryClient();
  const query = useQuery({
    queryKey: ['knowledge'],
    queryFn: () => listKnowledge(),
    refetchInterval: 5000,
  });
  const upload = useMutation({
    mutationFn: (file: File) => uploadKnowledge(file),
    onSuccess: () => {
      message.success('文件已安全保存，正在建立索引');
      queryClient.invalidateQueries({ queryKey: ['knowledge'] });
    },
    onError: () => message.error('上传失败，请检查文件格式和容量'),
  });
  const remove = useMutation({
    mutationFn: (id: string) => deleteKnowledge(id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['knowledge'] }),
  });
  const quota = query.data?.quota;
  const percent = quota ? Math.min(100, (quota.used_bytes / quota.limit_bytes) * 100) : 0;
  return (
    <div className="knowledge-page">
      <Typography.Title level={2}>个人知识库</Typography.Title>
      <Alert
        showIcon
        type="info"
        message="仅支持 Markdown 或 Markdown ZIP"
        description="ZIP 仅解析 Markdown、PNG 和 JPG，其他类型条目会被跳过。所有资料只对当前账号可见，每个账号上限 3 GiB。"
      />
      <Card title="上传资料">
        <Upload.Dragger
          accept=".md,.zip"
          multiple={false}
          showUploadList={false}
          disabled={upload.isPending}
          beforeUpload={(file) => {
            upload.mutate(file);
            return false;
          }}
        >
          <p className="ant-upload-drag-icon">
            <InboxOutlined />
          </p>
          <p>拖放或点击选择 .md / .zip 文件</p>
        </Upload.Dragger>
        {quota && (
          <div className="knowledge-quota">
            <Space>
              <span>已用 {(quota.used_bytes / 1024 / 1024).toFixed(1)} MiB</span>
              <span>剩余 {(quota.remaining_bytes / 1024 / 1024).toFixed(1)} MiB</span>
            </Space>
            <Progress percent={Number(percent.toFixed(1))} showInfo={false} />
          </div>
        )}
      </Card>
      <Card title="知识文档">
        <Table
          rowKey="id"
          loading={query.isLoading}
          dataSource={query.data?.items ?? []}
          locale={{ emptyText: <Empty description="还没有知识文档" /> }}
          columns={[
            { title: '标题', dataIndex: 'Title' },
            {
              title: '来源',
              dataIndex: 'SourceType',
              render: (v) => (v === 'note' ? '个人笔记' : '上传资料'),
            },
            {
              title: '大小',
              dataIndex: 'size_bytes',
              render: (v) => `${(v / 1024).toFixed(1)} KiB`,
            },
            {
              title: '状态',
              dataIndex: 'Status',
              render: (v, row) => (
                <Space direction="vertical" size={0}>
                  <Tag color={v === 'ready' ? 'success' : v === 'failed' ? 'error' : 'processing'}>
                    {statusText[v] ?? v}
                  </Tag>
                  {v === 'ready' && ['queued', 'running'].includes(row.index_job_status ?? '') && (
                    <Typography.Text type="secondary">可用，正在更新索引</Typography.Text>
                  )}
                  {v === 'ready' && row.index_job_status === 'failed' && (
                    <Typography.Text type="warning">
                      旧版本可用，最近更新失败：
                      {row.last_index_failure_code ?? 'KNOWLEDGE_INDEX_FAILED'}
                    </Typography.Text>
                  )}
                  {row.failure_summary && (
                    <Typography.Text type="danger">{row.failure_summary}</Typography.Text>
                  )}
                </Space>
              ),
            },
            {
              title: '操作',
              render: (_, row) => (
                <Popconfirm title="删除此知识文档？" onConfirm={() => remove.mutate(row.id)}>
                  <Button danger type="text" icon={<DeleteOutlined />}>
                    删除
                  </Button>
                </Popconfirm>
              ),
            },
          ]}
        />
      </Card>
    </div>
  );
}
