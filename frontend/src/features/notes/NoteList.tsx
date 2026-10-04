import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Alert, Button, Card, DatePicker, Empty, Input, Pagination, Select, Space } from 'antd';
import dayjs, { Dayjs } from 'dayjs';
import { useNavigate } from 'react-router-dom';
import { createNote, listNotes, listTags, searchNotes, type Note } from '../../api/notes';
import './Notes.css';
import UsageGuide from '../../components/UsageGuide';

export default function NoteList() {
  const navigate = useNavigate(),
    client = useQueryClient();
  const [page, setPage] = useState(1);
  const [type, setType] = useState<Note['type'] | ''>('');
  const [keyword, setKeyword] = useState('');
  const [tag, setTag] = useState<number>();
  const tags = useQuery({ queryKey: ['tags'], queryFn: listTags });
  const [date, setDate] = useState<Dayjs | null>(null);
  const selectedDate = date?.format('YYYY-MM-DD');
  const query = useQuery<{
    items: {
      id: number;
      title: string;
      content: string;
      note_date: string | null;
      word_count?: number;
    }[];
    total: number;
  }>({
    queryKey: ['notes', page, selectedDate, type, keyword, tag],
    queryFn: () =>
      keyword.trim()
        ? searchNotes({
            q: keyword.trim(),
            type,
            tag_id: tag,
            start_date: selectedDate,
            end_date: selectedDate,
            limit: 100,
          }).then((result) => ({
            ...result,
            items: result.items.map((n) => ({ ...n, content: n.snippet, word_count: undefined })),
          }))
        : listNotes({
            page,
            page_size: 12,
            type: type || undefined,
            tag_id: tag,
            start_date: selectedDate,
            end_date: selectedDate,
          }),
  });
  const create = useMutation({
    mutationFn: () =>
      createNote({
        type: type || 'normal',
        title: '未命名笔记',
        content: '',
        note_date: dayjs().format('YYYY-MM-DD'),
      }),
    onSuccess: (n) => {
      client.invalidateQueries({ queryKey: ['notes'] });
      navigate(`/notes/${n.id}`);
    },
  });
  return (
    <section className="notes-page">
      <header className="notes-toolbar">
        <h1>笔记本</h1>
        <Button
          onClick={() => {
            navigate('/notes/templates');
          }}
        >
          模板广场
        </Button>
        <DatePicker
          allowClear
          placeholder="按日期筛选"
          value={date}
          onChange={(value) => {
            setDate(value);
            setPage(1);
          }}
        />
        <Button type="primary" onClick={() => create.mutate()} loading={create.isPending}>
          新建笔记
        </Button>
      </header>
      <Space wrap style={{ marginBottom: 16 }}>
        <Input.Search
          aria-label="搜索笔记"
          placeholder="搜索标题和正文"
          onSearch={(value) => {
            setKeyword(value);
            setPage(1);
          }}
          allowClear
        />
        <Select
          aria-label="笔记类型"
          value={type}
          style={{ width: 130 }}
          onChange={(value) => {
            setType(value);
            setPage(1);
          }}
          options={[
            { value: '', label: '全部类型' },
            { value: 'normal', label: '普通笔记' },
            { value: 'daily', label: '日报' },
            { value: 'weekly', label: '周报' },
            { value: 'monthly', label: '月报' },
          ]}
        />
        <Select
          aria-label="筛选标签"
          placeholder="标签"
          allowClear
          style={{ width: 150 }}
          value={tag}
          options={tags.data?.map((t) => ({ value: t.id, label: t.name }))}
          onChange={(value) => {
            setTag(value);
            setPage(1);
          }}
        />
        <UsageGuide
          id="notes"
          steps={[
            { title: '记录', description: '新建普通笔记或选择日报、周报、月报。' },
            { title: '查找', description: '按关键词、标签、类型和日期筛选。' },
            { title: '保护内容', description: '保存前可查看差异；编辑页提供草稿恢复和历史版本。' },
          ]}
        />
      </Space>
      {query.isError && (
        <Alert
          type="error"
          message="笔记加载失败"
          action={<Button onClick={() => query.refetch()}>重试</Button>}
        />
      )}
      {keyword && <p>搜索显示最多 100 条匹配记录，请用标签和日期缩小范围。</p>}
      <div className="notes-list">
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
                {n.note_date || '无日期'}
                {n.word_count === undefined ? '' : ` · ${n.word_count} 字`}
              </div>
              <p>{n.content.slice(0, 120)}</p>
            </Card>
          ))
        ) : (
          <Empty />
        )}
      </div>
      {!keyword && (
        <Pagination
          className="notes-pagination"
          current={page}
          pageSize={12}
          total={query.data?.total || 0}
          onChange={setPage}
        />
      )}
    </section>
  );
}
