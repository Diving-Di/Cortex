import { useEffect, useRef, useState } from 'react';
import CodeMirror from '@uiw/react-codemirror';
import { markdown } from '@codemirror/lang-markdown';
import { Alert, Button, DatePicker, Input, Space, Spin, Tabs, Upload, message } from 'antd';
import dayjs from 'dayjs';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { useNavigate, useParams } from 'react-router-dom';
import { http } from '../../api/http';
import { getNote, saveNote } from '../../api/notes';
import { useTheme } from '../../app/theme';
import SafeMarkdown from '../../components/SafeMarkdown';
import './Notes.css';

type State = 'saved' | 'unsaved' | 'saving' | 'error' | 'conflict';
export default function NoteEditor() {
  const { resolved } = useTheme();
  const id = Number(useParams().id),
    navigate = useNavigate(),
    qc = useQueryClient();
  const query = useQuery({
    queryKey: ['note', id],
    queryFn: () => getNote(id),
  });
  const attachments = useQuery({
    queryKey: ['attachments', id],
    queryFn: async () => (await http.get<any[]>(`/api/v1/attachments/note/${id}`, {})).data,
  });
  const [title, setTitle] = useState(''),
    [content, setContent] = useState(''),
    [noteDate, setNoteDate] = useState(''),
    [updatedAt, setUpdatedAt] = useState(''),
    [state, setState] = useState<State>('saved');
  const initialized = useRef(false);
  const initialValues = useRef({ title: '', content: '', noteDate: '' });
  useEffect(() => {
    if (query.data && !initialized.current) {
      setTitle(query.data.title);
      setContent(query.data.content);
      setNoteDate(query.data.note_date || '');
      setUpdatedAt(query.data.updated_at);
      initialValues.current = {
        title: query.data.title,
        content: query.data.content,
        noteDate: query.data.note_date || '',
      };
      initialized.current = true;
    }
  }, [query.data]);
  async function persist() {
    if (!initialized.current || state === 'saving') return;
    setState('saving');
    try {
      const n = await saveNote(id, {
        title,
        content,
        note_date: noteDate || null,
        expected_updated_at: updatedAt,
      });
      setUpdatedAt(n.updated_at);
      setState('saved');
      qc.setQueryData(['note', id], n);
      await qc.invalidateQueries({ queryKey: ['notes'] });
      navigate('/notes/list');
    } catch (e: any) {
      setState(e?.response?.status === 409 ? 'conflict' : 'error');
    }
  }
  useEffect(() => {
    if (!initialized.current) return;
    const initial = initialValues.current;
    setState(
      title === initial.title && content === initial.content && noteDate === initial.noteDate
        ? 'saved'
        : 'unsaved',
    );
  }, [title, content, noteDate]);
  useEffect(() => {
    const handler = (e: BeforeUnloadEvent) => {
      if (state !== 'saved') {
        e.preventDefault();
        e.returnValue = '';
      }
    };
    window.addEventListener('beforeunload', handler);
    return () => window.removeEventListener('beforeunload', handler);
  }, [state]);
  if (query.isLoading) return <Spin />;
  if (!query.data) return <Alert type="error" message="笔记不存在" />;
  return (
    <section>
      <header className="note-editor-toolbar">
        <Space size="middle">
          <Button
            className="note-editor-action"
            disabled={state === 'saving'}
            onClick={() => navigate('/notes/list')}
          >
            取消
          </Button>
          <Button
            className="note-editor-action"
            type="primary"
            loading={state === 'saving'}
            disabled={state === 'saved'}
            onClick={persist}
          >
            {state === 'error' || state === 'conflict' ? '重试保存' : '保存'}
          </Button>
          <span>
            状态：
            {
              {
                saved: '已保存',
                unsaved: '未保存',
                saving: '保存中',
                error: '保存失败',
                conflict: '内容冲突',
              }[state]
            }
          </span>
        </Space>
      </header>
      {state === 'conflict' && (
        <Alert type="warning" message="服务器内容已更新，请刷新后合并，避免覆盖。" />
      )}
      <Input
        disabled={state === 'saving'}
        value={title}
        onChange={(e) => setTitle(e.target.value)}
        size="large"
        style={{ margin: '12px 0' }}
      />
      <DatePicker
        allowClear={false}
        disabled={state === 'saving'}
        value={noteDate ? dayjs(noteDate) : null}
        onChange={(value) => value && setNoteDate(value.format('YYYY-MM-DD'))}
        style={{ marginBottom: 12 }}
      />
      <Upload
        disabled={state === 'saving'}
        multiple
        showUploadList={false}
        customRequest={async (o) => {
          const form = new FormData();
          form.append('file', o.file as Blob);
          try {
            const r = await fetch(`/api/v1/attachments?note_id=${id}`, {
              method: 'POST',
              body: form,
            });
            if (!r.ok) throw new Error('upload failed');
            message.success('附件已上传');
            attachments.refetch();
            o.onSuccess?.({});
          } catch (e) {
            o.onError?.(e as Error);
          }
        }}
      >
        <Button>上传附件</Button>
      </Upload>
      <Space wrap>
        {attachments.data?.map((a) => (
          <span key={a.id}>
            {a.original_name}{' '}
            <Button
              size="small"
              onClick={async () => {
                const r = await fetch(`/api/v1/attachments/${a.id}`, {});
                const blob = await r.blob();
                const url = URL.createObjectURL(blob);
                const link = document.createElement('a');
                link.href = url;
                link.download = a.original_name;
                link.click();
                URL.revokeObjectURL(url);
              }}
            >
              下载
            </Button>
            <Button
              size="small"
              danger
              onClick={() =>
                http.delete(`/api/v1/attachments/${a.id}`, {}).then(() => attachments.refetch())
              }
            >
              删除
            </Button>
          </span>
        ))}
      </Space>
      <Tabs
        items={[
          {
            key: 'edit',
            label: '编辑',
            children: (
              <CodeMirror
                editable={state !== 'saving'}
                value={content}
                height="60vh"
                theme={resolved}
                extensions={[markdown()]}
                onChange={setContent}
              />
            ),
          },
          {
            key: 'preview',
            label: '预览',
            children: <SafeMarkdown>{content}</SafeMarkdown>,
          },
        ]}
      />
    </section>
  );
}
