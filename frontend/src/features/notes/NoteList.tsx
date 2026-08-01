import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Button, Card, DatePicker, Empty, Pagination } from 'antd';
import dayjs, { Dayjs } from 'dayjs';
import { useNavigate } from 'react-router-dom';
import { createNote, listNotes } from '../../api/notes';
import './Notes.css';

export default function NoteList({ token }: { token: string }) {
  const navigate = useNavigate(),
    client = useQueryClient();
  const [page, setPage] = useState(1);
  const [date, setDate] = useState<Dayjs | null>(null);
  const selectedDate = date?.format('YYYY-MM-DD');
  const query = useQuery({
    queryKey: ['notes', page, selectedDate],
    queryFn: () =>
      listNotes(token, {
        page,
        page_size: 12,
        type: 'normal',
        start_date: selectedDate,
        end_date: selectedDate,
      }),
  });
  const create = useMutation({
    mutationFn: () =>
      createNote(token, {
        type: 'normal',
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
            localStorage.setItem('diary:notes-section', 'templates');
            navigate('/notes');
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
                {n.note_date || '无日期'} · {n.word_count} 字
              </div>
              <p>{n.content.slice(0, 120)}</p>
            </Card>
          ))
        ) : (
          <Empty />
        )}
      </div>
      <Pagination
        className="notes-pagination"
        current={page}
        pageSize={12}
        total={query.data?.total || 0}
        onChange={setPage}
      />
    </section>
  );
}
