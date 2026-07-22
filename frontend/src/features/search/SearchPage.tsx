import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Card, Input, Select, Space } from 'antd';
import { useNavigate } from 'react-router-dom';
import { authHeaders, http } from '../../api/http';

export default function SearchPage() {
  const [q, setQ] = useState(''),
    [type, setType] = useState<string>();
  const navigate = useNavigate();
  const token = localStorage.getItem('token') || '';
  const result = useQuery({
    queryKey: ['search', q, type],
    queryFn: async () =>
      (
        await http.get('/api/v1/search', {
          headers: authHeaders(token),
          params: { q, type },
        })
      ).data,
    enabled: q.length > 0,
  });
  const highlight = (s: string) => {
    const i = s.toLowerCase().indexOf(q.toLowerCase());
    return i < 0 ? (
      s
    ) : (
      <>
        {s.slice(0, i)}
        <mark>{s.slice(i, i + q.length)}</mark>
        {s.slice(i + q.length)}
      </>
    );
  };
  return (
    <section>
      <h1>搜索</h1>
      <Space>
        <Input.Search allowClear placeholder="搜索标题和正文" onSearch={setQ} />
        <Select
          allowClear
          placeholder="类型"
          onChange={setType}
          options={['normal', 'daily', 'weekly', 'monthly'].map((v) => ({
            value: v,
            label: v,
          }))}
        />
      </Space>
      {result.data?.items.map((x: any) => (
        <Card
          key={x.id}
          title={highlight(x.title)}
          onClick={() => navigate(`/notes/${x.id}`)}
          hoverable
          style={{ marginTop: 12 }}
        >
          {highlight(x.snippet)}
        </Card>
      ))}
    </section>
  );
}
