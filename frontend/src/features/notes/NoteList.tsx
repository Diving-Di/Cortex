import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Button, Card, DatePicker, Empty, Pagination, Select, Space } from 'antd';
import { useNavigate } from 'react-router-dom';
import { createNote, listNotes, listTags } from '../../api/notes';

export default function NoteList({ token }: { token: string }) {
  const navigate = useNavigate(),
    client = useQueryClient();
  const [page, setPage] = useState(1);
  const [type, setType] = useState<string>();
  const [tagId, setTagId] = useState<number>();
  const query = useQuery({
    queryKey: ['notes', page, type, tagId],
    queryFn: () => listNotes(token, { page, page_size: 12, type, tag_id: tagId }),
  });
  const tags = useQuery({ queryKey: ['tags'], queryFn: () => listTags(token) });
  const create = useMutation({
    mutationFn: () => createNote(token, { type: 'normal', title: '未命名笔记', content: '' }),
    onSuccess: (n) => {
      client.invalidateQueries({ queryKey: ['notes'] });
      navigate(`/notes/${n.id}`);
    },
  });
  return (
    <section>
      <Space>
        <h1>笔记本</h1>
        <Button type="primary" onClick={() => create.mutate()} loading={create.isPending}>
          新建笔记
        </Button>
        <Select
          allowClear
          placeholder="类型"
          style={{ width: 140 }}
          value={type}
          onChange={setType}
          options={['normal', 'daily', 'weekly', 'monthly'].map((v) => ({
            value: v,
            label: v,
          }))}
        />
        <Select
          allowClear
          placeholder="标签"
          style={{ width: 200 }}
          value={tagId}
          onChange={setTagId}
          options={tags.data?.map((t) => ({ value: t.id, label: t.name }))}
        />
      </Space>
      {query.data?.items.length ? (
        query.data.items.map((n) => (
          <Card
            key={n.id}
            hoverable
            onClick={() => navigate(`/notes/${n.id}`)}
            title={n.title}
            style={{ marginBottom: 12 }}
          >
            <div>
              {n.type} · {n.note_date || '无日期'} · {n.word_count} 字
            </div>
            <p>{n.content.slice(0, 120)}</p>
          </Card>
        ))
      ) : (
        <Empty />
      )}
      <Pagination current={page} pageSize={12} total={query.data?.total || 0} onChange={setPage} />
    </section>
  );
}
