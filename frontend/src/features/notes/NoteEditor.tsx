import { useContext, useEffect, useRef, useState } from 'react';
import CodeMirror from '@uiw/react-codemirror';
import { markdown } from '@codemirror/lang-markdown';
import {
  Alert,
  Button,
  Card,
  DatePicker,
  Input,
  List,
  Modal,
  Select,
  Space,
  Spin,
  Tabs,
  Upload,
  message,
} from 'antd';
import dayjs from 'dayjs';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { UNSAFE_DataRouterContext, useNavigate, useParams } from 'react-router-dom';
import { http } from '../../api/http';
import {
  getNote,
  saveNote,
  listNotes,
  listRevisions,
  restoreRevision,
  listTags,
  noteTags,
  setNoteTags,
  type Note,
  type Revision,
} from '../../api/notes';
import { useTheme } from '../../app/theme';
import SafeMarkdown from '../../components/SafeMarkdown';
import './Notes.css';
import { useDraftScope, readDraft, writeDraft, removeDraft } from '../../app/drafts';
import ContentDiff from '../../components/ContentDiff';
import LeaveGuard from '../../components/LeaveGuard';

type State = 'saved' | 'unsaved' | 'saving' | 'error' | 'conflict';
export default function NoteEditor() {
  const dataRouter = useContext(UNSAFE_DataRouterContext);
  const scope = useDraftScope();
  const [loadedID, setLoadedID] = useState<number>();
  const [recovered, setRecovered] = useState(false);
  const [storageFailed, setStorageFailed] = useState(false);
  const [serverNote, setServerNote] = useState<Note>();
  const [historyOpen, setHistoryOpen] = useState(false);
  const [selectedRevision, setSelectedRevision] = useState<Revision>();
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
  useEffect(() => {
    initialized.current = false;
    setRecovered(false);
    setSelectedRevision(undefined);
    setServerNote(undefined);
  }, [id]);
  const tags = useQuery({ queryKey: ['tags'], queryFn: listTags });
  const assigned = useQuery({ queryKey: ['note-tags', id], queryFn: () => noteTags(id) });
  const revisions = useQuery({
    queryKey: ['revisions', id],
    queryFn: () => listRevisions(id),
    enabled: historyOpen,
  });
  const related = useQuery({
    queryKey: ['related-notes', id, assigned.data?.[0]?.id],
    queryFn: () =>
      listNotes(
        assigned.data?.length
          ? { tag_id: assigned.data[0].id, page_size: 6 }
          : {
              start_date: dayjs().subtract(7, 'day').format('YYYY-MM-DD'),
              end_date: dayjs().format('YYYY-MM-DD'),
              page_size: 6,
            },
      ),
  });
  const initialValues = useRef({ title: '', content: '', noteDate: '' });
  const draftKey = `note:${id}`;
  useEffect(() => {
    if (query.data && query.data.id === id && !initialized.current) {
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
      setLoadedID(id);
      const draft = readDraft<{
        title: string;
        content: string;
        noteDate: string;
        updatedAt: string;
      }>(scope, draftKey);
      if (draft?.value && typeof draft.value.content === 'string') {
        setTitle(draft.value.title);
        setContent(draft.value.content);
        setNoteDate(draft.value.noteDate);
        setUpdatedAt(draft.value.updatedAt);
        setRecovered(true);
        if (draft.value.updatedAt !== query.data.updated_at) {
          setServerNote(query.data);
          setState('conflict');
        }
      }
    }
  }, [query.data, id, scope, draftKey]);
  async function persist() {
    if (!initialized.current || loadedID !== id || state === 'saving') return;
    setState('saving');
    try {
      const n = await saveNote(id, {
        title,
        content,
        note_date: noteDate || null,
        expected_updated_at: updatedAt,
      });
      setUpdatedAt(n.updated_at);
      removeDraft(scope, draftKey);
      initialValues.current = { title: n.title, content: n.content, noteDate: n.note_date || '' };
      setState('saved');
      qc.setQueryData(['note', id], n);
      await qc.invalidateQueries({ queryKey: ['notes'] });
      navigate('/notes/list');
    } catch (e: any) {
      setState(e?.response?.status === 409 ? 'conflict' : 'error');
      if (e?.response?.status === 409) {
        try {
          setServerNote(await getNote(id));
        } catch {
          message.error('最新内容加载失败，草稿已保留');
        }
      }
    }
  }
  useEffect(() => {
    if (!initialized.current || loadedID !== id) return;
    const initial = initialValues.current;
    if (state === 'saving' || serverNote) return;
    setState(
      title === initial.title && content === initial.content && noteDate === initial.noteDate
        ? 'saved'
        : 'unsaved',
    );
  }, [title, content, noteDate, serverNote, loadedID, id]);
  useEffect(() => {
    if (!initialized.current || loadedID !== id || state === 'saved' || state === 'saving') return;
    setStorageFailed(
      !writeDraft(scope, {
        key: draftKey,
        label: title || '未命名笔记',
        path: `/notes/${id}`,
        value: { title, content, noteDate, updatedAt },
      }),
    );
  }, [title, content, noteDate, updatedAt, state, scope, draftKey, id, loadedID]);
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
      {dataRouter && <LeaveGuard dirty={state !== 'saved' && state !== 'saving'} />}
      {recovered && <Alert type="info" message="已恢复当前标签页的未保存草稿" />}
      {storageFailed && (
        <Alert type="warning" message="浏览器草稿存储不可用，请及时保存或复制内容" />
      )}
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
            disabled={state === 'saved' || state === 'conflict'}
            onClick={persist}
          >
            {state === 'error' || state === 'conflict' ? '重试保存' : '保存'}
          </Button>
          <Button onClick={() => setHistoryOpen(true)}>历史版本</Button>
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
        <Alert
          type="warning"
          message="服务器内容已更新，草稿已保留。请比较并编辑合并，再确认使用最新版本保存。"
        />
      )}
      {serverNote && (
        <Card title="冲突内容比较">
          <ContentDiff before={serverNote.content} after={content} />
          <p>
            服务器标题：{serverNote.title}；日期：{serverNote.note_date}
          </p>
          <Button
            onClick={() => {
              initialValues.current = {
                title: serverNote.title,
                content: serverNote.content,
                noteDate: serverNote.note_date || '',
              };
              setUpdatedAt(serverNote.updated_at);
              setServerNote(undefined);
              setState('unsaved');
            }}
          >
            我已合并，使用最新版本
          </Button>
        </Card>
      )}
      <Select
        mode="multiple"
        aria-label="笔记标签"
        placeholder="设置标签"
        style={{ width: '100%', marginTop: 12 }}
        value={assigned.data?.map((t) => t.id) || []}
        options={tags.data?.map((t) => ({ value: t.id, label: t.name }))}
        onChange={async (ids) => {
          try {
            await setNoteTags(id, ids);
            await assigned.refetch();
          } catch {
            message.error('标签保存失败');
          }
        }}
      />
      <Input
        aria-label="笔记标题"
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
      <Card title={assigned.data?.length ? '同标签笔记' : '最近一周回顾'} style={{ marginTop: 16 }}>
        <List
          dataSource={related.data?.items.filter((n) => n.id !== id) || []}
          renderItem={(n) => (
            <List.Item>
              <a href={`/notes/${n.id}`}>{n.title}</a>
            </List.Item>
          )}
        />
      </Card>
      <Modal
        title="历史版本"
        open={historyOpen}
        onCancel={() => setHistoryOpen(false)}
        footer={null}
        width={900}
      >
        {revisions.isError && (
          <Alert
            type="error"
            message="历史版本加载失败"
            action={<Button onClick={() => revisions.refetch()}>重试</Button>}
          />
        )}
        <List
          loading={revisions.isLoading}
          dataSource={revisions.data || []}
          renderItem={(revision) => (
            <List.Item>
              <Button onClick={() => setSelectedRevision(revision)}>
                {new Date(revision.created_at).toLocaleString()} · {revision.reason}
              </Button>
            </List.Item>
          )}
        />
        {selectedRevision && (
          <>
            <ContentDiff before={content} after={selectedRevision.content} />
            <Button
              type="primary"
              disabled={state === 'saving'}
              onClick={async () => {
                if (
                  !window.confirm(
                    '恢复该版本正文？未保存编辑会被丢弃，服务器当前正文会保留为历史版本。',
                  )
                )
                  return;
                try {
                  const n = await restoreRevision(id, selectedRevision.id, updatedAt);
                  setContent(n.content);
                  setTitle(n.title);
                  setNoteDate(n.note_date || '');
                  setUpdatedAt(n.updated_at);
                  initialValues.current = {
                    title: n.title,
                    content: n.content,
                    noteDate: n.note_date || '',
                  };
                  removeDraft(scope, draftKey);
                  setState('saved');
                  setServerNote(undefined);
                  setHistoryOpen(false);
                  qc.setQueryData(['note', id], n);
                  await qc.invalidateQueries({ queryKey: ['revisions', id] });
                  await qc.invalidateQueries({ queryKey: ['notes'] });
                } catch {
                  try {
                    setServerNote(await getNote(id));
                  } catch {
                    /* Preserve local content if refresh also fails. */
                  }
                  setState('conflict');
                  message.error('恢复失败，草稿已保留，请检查最新版本');
                }
              }}
            >
              确认恢复正文
            </Button>
          </>
        )}
      </Modal>
    </section>
  );
}
