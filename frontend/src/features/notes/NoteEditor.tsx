import { useEffect, useRef, useState } from 'react'
import CodeMirror from '@uiw/react-codemirror'
import { markdown } from '@codemirror/lang-markdown'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { Alert, Button, Input, Select, Space, Spin, Tabs, Upload, message } from 'antd'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate, useParams } from 'react-router-dom'
import { authHeaders, http } from '../../api/http'
import { getNote, listTags, noteTags, saveNote, setNoteTags } from '../../api/notes'

type State='saved'|'unsaved'|'saving'|'error'|'conflict'
export default function NoteEditor({token}:{token:string}) {
  const id=Number(useParams().id), navigate=useNavigate(), qc=useQueryClient(); const query=useQuery({queryKey:['note',id],queryFn:()=>getNote(token,id)}); const tags=useQuery({queryKey:['tags'],queryFn:()=>listTags(token)}); const assigned=useQuery({queryKey:['note-tags',id],queryFn:()=>noteTags(token,id)})
  const attachments=useQuery({queryKey:['attachments',id],queryFn:async()=> (await http.get<any[]>(`/api/v1/attachments/note/${id}`,{headers:authHeaders(token)})).data})
  const [title,setTitle]=useState(''),[content,setContent]=useState(''),[updatedAt,setUpdatedAt]=useState(''),[state,setState]=useState<State>('saved'); const initialized=useRef(false); const timer=useRef<number>()
  useEffect(()=>{if(query.data&&!initialized.current){setTitle(query.data.title);setContent(query.data.content);setUpdatedAt(query.data.updated_at);initialized.current=true}},[query.data])
  async function persist(){if(!initialized.current)return;setState('saving');try{const n=await saveNote(token,id,{title,content,expected_updated_at:updatedAt});setUpdatedAt(n.updated_at);setState('saved');qc.invalidateQueries({queryKey:['notes']})}catch(e:any){setState(e?.response?.status===409?'conflict':'error')}}
  useEffect(()=>{if(!initialized.current)return;setState('unsaved');window.clearTimeout(timer.current);timer.current=window.setTimeout(persist,800);return()=>window.clearTimeout(timer.current)},[title,content])
  useEffect(()=>{const handler=(e:BeforeUnloadEvent)=>{if(state==='unsaved'||state==='saving'||state==='error'){e.preventDefault();e.returnValue=''}};window.addEventListener('beforeunload',handler);return()=>window.removeEventListener('beforeunload',handler)},[state])
  if(query.isLoading)return <Spin/>; if(!query.data)return <Alert type="error" message="笔记不存在"/>
  return <section><Space><Button onClick={()=>{if(state==='saved'||confirm('仍有未保存内容，确认离开？'))navigate('/notes')}}>返回</Button><span>状态：{{saved:'已保存',unsaved:'未保存',saving:'保存中',error:'保存失败',conflict:'内容冲突'}[state]}</span>{(state==='error'||state==='conflict')&&<Button onClick={persist}>重试保存</Button>}</Space>
    {state==='conflict'&&<Alert type="warning" message="服务器内容已更新，请刷新后合并，避免覆盖。"/>}<Input value={title} onChange={e=>setTitle(e.target.value)} size="large" style={{margin:'12px 0'}}/>
    <Select mode="multiple" style={{width:'100%',marginBottom:12}} placeholder="标签" value={assigned.data?.map(t=>t.id)} options={tags.data?.map(t=>({value:t.id,label:t.name}))} onChange={ids=>setNoteTags(token,id,ids).then(()=>assigned.refetch())}/>
    <Upload multiple showUploadList={false} customRequest={async o=>{const form=new FormData();form.append('file',o.file as Blob);try{const r=await fetch(`/api/v1/attachments?note_id=${id}`,{method:'POST',headers:authHeaders(token),body:form});if(!r.ok)throw new Error('upload failed');message.success('附件已上传');attachments.refetch();o.onSuccess?.({})}catch(e){o.onError?.(e as Error)}}}><Button>上传附件</Button></Upload>
    <Space wrap>{attachments.data?.map(a=><span key={a.id}>{a.original_name} <Button size="small" onClick={async()=>{const r=await fetch(`/api/v1/attachments/${a.id}`,{headers:authHeaders(token)});const blob=await r.blob();const url=URL.createObjectURL(blob);const link=document.createElement('a');link.href=url;link.download=a.original_name;link.click();URL.revokeObjectURL(url)}}>下载</Button><Button size="small" danger onClick={()=>http.delete(`/api/v1/attachments/${a.id}`,{headers:authHeaders(token)}).then(()=>attachments.refetch())}>删除</Button></span>)}</Space>
    <Tabs items={[{key:'edit',label:'编辑',children:<CodeMirror value={content} height="60vh" extensions={[markdown()]} onChange={setContent}/>},{key:'preview',label:'预览',children:<article className="markdown-preview"><ReactMarkdown remarkPlugins={[remarkGfm]}>{content}</ReactMarkdown></article>}]} /></section>
}
