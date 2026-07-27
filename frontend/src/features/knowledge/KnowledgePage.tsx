import { useMemo, useState } from 'react';
import {
  Button,
  Card,
  Empty,
  Form,
  Input,
  Modal,
  Popconfirm,
  Select,
  Space,
  Table,
  Tag,
  Typography,
  Upload,
  message,
} from 'antd';
import { DeleteOutlined, FolderAddOutlined, InboxOutlined } from '@ant-design/icons';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import type { UploadProps } from 'antd';
import {
  createKnowledgeCollection,
  deleteKnowledgeDocument,
  listKnowledgeCollections,
  listKnowledgeDocuments,
  uploadKnowledgeDocument,
} from '../../api/knowledge';
import type { KnowledgeDocument } from '../../api/knowledge';
import './KnowledgePage.css';

interface Props {
  token: string;
}

const statusLabels: Record<KnowledgeDocument['status'], { label: string; color: string }> = {
  uploaded: { label: '等待处理', color: 'default' },
  extracting: { label: '正在解析', color: 'processing' },
  indexing: { label: '正在索引', color: 'processing' },
  ready: { label: '可用于问答', color: 'success' },
  failed: { label: '处理失败', color: 'error' },
  deleting: { label: '正在删除', color: 'warning' },
};

export default function KnowledgePage({ token }: Props) {
  const queryClient = useQueryClient();
  const [collectionId, setCollectionId] = useState<number>();
  const [collectionOpen, setCollectionOpen] = useState(false);
  const [form] = Form.useForm();
  const collections = useQuery({
    queryKey: ['knowledge-collections'],
    queryFn: () => listKnowledgeCollections(token),
  });
  const documents = useQuery({
    queryKey: ['knowledge-documents', collectionId],
    queryFn: () => listKnowledgeDocuments(token, collectionId),
    refetchInterval: (query) =>
      query.state.data?.items.some((item) =>
        ['uploaded', 'extracting', 'indexing'].includes(item.status),
      )
        ? 3000
        : false,
  });
  const createCollection = useMutation({
    mutationFn: (value: { name: string; description?: string }) =>
      createKnowledgeCollection(token, value),
    onSuccess: async () => {
      setCollectionOpen(false);
      form.resetFields();
      await queryClient.invalidateQueries({ queryKey: ['knowledge-collections'] });
    },
  });
  const removeDocument = useMutation({
    mutationFn: (id: number) => deleteKnowledgeDocument(token, id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['knowledge-documents'] }),
  });
  const uploadProps: UploadProps = {
    accept: '.txt,.pdf,.docx',
    multiple: true,
    showUploadList: false,
    customRequest: async ({ file, onSuccess, onError }) => {
      try {
        await uploadKnowledgeDocument(token, file as File, collectionId);
        onSuccess?.({});
        message.success(`${(file as File).name} 已上传，正在建立索引`);
        await queryClient.invalidateQueries({ queryKey: ['knowledge-documents'] });
      } catch (error) {
        onError?.(error as Error);
        message.error(`${(file as File).name} 上传失败`);
      }
    },
  };
  const collectionOptions = useMemo(
    () => (collections.data || []).map((item) => ({ label: item.name, value: item.id })),
    [collections.data],
  );

  return (
    <div className="knowledge-page">
      <div className="knowledge-header">
        <div>
          <Typography.Title level={2}>个人知识库</Typography.Title>
          <Typography.Text type="secondary">
            上传中英文 TXT、文本型 PDF 或 DOCX，Cortex 会在本地建立父子索引。
          </Typography.Text>
        </div>
        <Button icon={<FolderAddOutlined />} onClick={() => setCollectionOpen(true)}>
          新建集合
        </Button>
      </div>

      <Card>
        <Space className="knowledge-toolbar" wrap>
          <Select
            allowClear
            placeholder="全部知识集合"
            options={collectionOptions}
            value={collectionId}
            onChange={setCollectionId}
            style={{ minWidth: 220 }}
          />
          <Typography.Text type="secondary">{documents.data?.total || 0} 个文件</Typography.Text>
        </Space>
        <Upload.Dragger {...uploadProps} className="knowledge-uploader">
          <p className="ant-upload-drag-icon">
            <InboxOutlined />
          </p>
          <p className="ant-upload-text">拖入资料，或点击选择文件</p>
          <p className="ant-upload-hint">支持 .txt、.pdf、.docx；扫描 PDF 暂不支持 OCR</p>
        </Upload.Dragger>
      </Card>

      <Card>
        <Table
          rowKey="id"
          loading={documents.isLoading}
          dataSource={documents.data?.items || []}
          locale={{ emptyText: <Empty description="尚未上传知识文件" /> }}
          columns={[
            { title: '文件名', dataIndex: 'original_name', ellipsis: true },
            {
              title: '状态',
              dataIndex: 'status',
              render: (status: KnowledgeDocument['status'], record: KnowledgeDocument) => (
                <Space direction="vertical" size={0}>
                  <Tag color={statusLabels[status].color}>{statusLabels[status].label}</Tag>
                  {record.error_message ? (
                    <Typography.Text type="danger">{record.error_message}</Typography.Text>
                  ) : null}
                </Space>
              ),
            },
            {
              title: '索引',
              render: (_, record: KnowledgeDocument) =>
                `${record.parent_chunk_count} 父块 / ${record.child_chunk_count} 子块`,
            },
            {
              title: '大小',
              dataIndex: 'size',
              render: (size: number) => `${(size / 1024 / 1024).toFixed(2)} MB`,
            },
            {
              title: '操作',
              render: (_, record: KnowledgeDocument) => (
                <Popconfirm
                  title="删除该知识文件？"
                  description="删除后将立即停止参与检索。"
                  onConfirm={() => removeDocument.mutate(record.id)}
                >
                  <Button danger type="text" icon={<DeleteOutlined />} aria-label="删除知识文件" />
                </Popconfirm>
              ),
            },
          ]}
        />
      </Card>

      <Modal
        title="新建知识集合"
        open={collectionOpen}
        onCancel={() => setCollectionOpen(false)}
        onOk={() => form.submit()}
        confirmLoading={createCollection.isPending}
      >
        <Form form={form} layout="vertical" onFinish={(value) => createCollection.mutate(value)}>
          <Form.Item name="name" label="名称" rules={[{ required: true }, { max: 120 }]}>
            <Input />
          </Form.Item>
          <Form.Item name="description" label="说明" rules={[{ max: 500 }]}>
            <Input.TextArea rows={3} />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
}
