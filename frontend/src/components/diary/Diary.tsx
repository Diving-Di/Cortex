import { useEffect, useRef, useState } from 'react'
import {
  Button,
  Card,
  Empty,
  Image,
  Input,
  Popconfirm,
  Spin,
  Upload,
  message as antdMessage,
} from 'antd'
import type { UploadFile } from 'antd'
import { DeleteOutlined, PictureOutlined, PlusOutlined } from '@ant-design/icons'
import type { DiaryEntry } from '../../types'
import { createDiaryEntry, deleteDiaryEntry, fetchDiaryEntries } from '../../api/diary'
import './Diary.css'

interface DiaryProps {
  token: string
}

function formatDate(value: string): string {
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return value
  return d.toLocaleString('zh-CN', { hour12: false })
}

export default function Diary({ token }: DiaryProps) {
  const [entries, setEntries] = useState<DiaryEntry[]>([])
  const [loading, setLoading] = useState(false)
  const [content, setContent] = useState('')
  const [fileList, setFileList] = useState<UploadFile[]>([])
  const [submitting, setSubmitting] = useState(false)
  const fileRef = useRef<File | null>(null)

  useEffect(() => {
    loadEntries()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  async function loadEntries() {
    setLoading(true)
    try {
      setEntries(await fetchDiaryEntries(token))
    } catch (e) {
      antdMessage.error('加载日记失败')
    } finally {
      setLoading(false)
    }
  }

  async function handleSubmit() {
    const text = content.trim()
    if (!text && !fileRef.current) {
      antdMessage.warning('请上传图片或写点什么')
      return
    }
    setSubmitting(true)
    try {
      await createDiaryEntry(token, text, fileRef.current)
      setContent('')
      setFileList([])
      fileRef.current = null
      await loadEntries()
      antdMessage.success('已发布')
    } catch (e) {
      antdMessage.error('发布失败')
    } finally {
      setSubmitting(false)
    }
  }

  async function handleDelete(id: number) {
    try {
      await deleteDiaryEntry(token, id)
      await loadEntries()
    } catch (e) {
      antdMessage.error('删除失败')
    }
  }

  return (
    <div className="diary">
      <Card className="diary-editor" title="写一篇轻日记">
        <Upload
          listType="picture-card"
          maxCount={1}
          fileList={fileList}
          beforeUpload={(file) => {
            fileRef.current = file
            setFileList([
              {
                uid: '-1',
                name: file.name,
                status: 'done',
                url: URL.createObjectURL(file),
              },
            ])
            return false
          }}
          onRemove={() => {
            fileRef.current = null
            setFileList([])
          }}
        >
          {fileList.length === 0 ? (
            <div>
              <PlusOutlined />
              <div style={{ marginTop: 8 }}>上传图片</div>
            </div>
          ) : null}
        </Upload>
        <Input.TextArea
          value={content}
          onChange={(e) => setContent(e.target.value)}
          placeholder="为这张图片写一段话，记录此刻的心情…"
          autoSize={{ minRows: 3, maxRows: 6 }}
        />
        <div className="diary-editor-actions">
          <Button type="primary" loading={submitting} onClick={handleSubmit}>
            发布日记
          </Button>
        </div>
      </Card>

      <div className="diary-feed">
        {loading ? (
          <div className="diary-loading">
            <Spin />
          </div>
        ) : entries.length === 0 ? (
          <Empty
            image={<PictureOutlined style={{ fontSize: 56, color: '#c9d1e6' }} />}
            description="还没有日记，发布第一篇吧"
          />
        ) : (
          entries.map((entry) => (
            <Card key={entry.id} className="diary-card">
              {entry.image ? (
                <Image src={entry.image} className="diary-card-image" />
              ) : null}
              {entry.content ? <p className="diary-card-content">{entry.content}</p> : null}
              <div className="diary-card-footer">
                <span className="diary-card-time">{formatDate(entry.created_at)}</span>
                <Popconfirm title="删除这篇日记？" onConfirm={() => handleDelete(entry.id)}>
                  <Button type="text" danger size="small" icon={<DeleteOutlined />} />
                </Popconfirm>
              </div>
            </Card>
          ))
        )}
      </div>
    </div>
  )
}
