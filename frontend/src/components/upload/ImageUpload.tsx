import { useEffect, useState } from 'react'
import { Button, Select, Upload, message } from 'antd'
import type { UploadProps } from 'antd'
import type { UploadFile } from 'antd/es/upload/interface'
import { InboxOutlined } from '@ant-design/icons'
import { uploadImage } from '../../api/images'
import { fetchCustomTagNames } from '../../api/tags'
import { normalizeTagNames } from '../../utils/tags'
import './ImageUpload.css'

const { Dragger } = Upload

interface ImageUploadProps {
  token: string
  onUploaded?: () => void
}

export default function ImageUpload({ token, onUploaded }: ImageUploadProps) {
  const [fileList, setFileList] = useState<UploadFile[]>([])
  const [loading, setLoading] = useState(false)
  const [tagNames, setTagNames] = useState<string[]>([])
  const [availableTagNames, setAvailableTagNames] = useState<string[]>([])

  useEffect(() => {
    if (token) {
      fetchTags()
      return
    }
    setAvailableTagNames([])
    setTagNames([])
  }, [token])

  async function fetchTags() {
    if (!token) return
    try {
      setAvailableTagNames(await fetchCustomTagNames(token))
    } catch (err) {
      console.error(err)
    }
  }

  async function upload() {
    if (fileList.length === 0) return

    setLoading(true)
    let successCount = 0
    let failCount = 0
    const cleaned = (tagNames || []).flatMap((x: string) => normalizeTagNames(x))

    const promises = fileList.map(async (file: UploadFile) => {
      if (!file.originFileObj) {
        failCount++
        return
      }
      try {
        await uploadImage(token, file.originFileObj, cleaned)
        successCount++
      } catch (err) {
        console.error(err)
        failCount++
      }
    })

    await Promise.all(promises)

    if (successCount > 0) {
      message.success(`成功上传 ${successCount} 张图片`)
      onUploaded?.()
    }

    if (failCount > 0) {
      message.error(`${failCount} 张图片上传失败`)
    } else {
      setFileList([])
      setTagNames([])
      fetchTags()
    }

    setLoading(false)
  }

  return (
    <div className="upload-container">
      <Dragger
        multiple
        beforeUpload={() => false}
        fileList={fileList}
        onChange={({ fileList: nextFileList }: Parameters<NonNullable<UploadProps['onChange']>>[0]) => {
          setFileList(nextFileList)
        }}
        onRemove={(file: UploadFile) => {
          setFileList((current: UploadFile[]) => current.filter((item: UploadFile) => item.uid !== file.uid))
          return true
        }}
      >
        <p className="ant-upload-drag-icon">
          <InboxOutlined />
        </p>
        <p className="ant-upload-text">将文件拖到此处，或点击上传</p>
      </Dragger>

      <div className="tag-area">
        <div className="tag-label">自定义标签（可多选/可输入）</div>
        <Select
          mode="tags"
          placeholder="输入标签后回车，或从下拉中选择"
          className="tag-select"
          value={tagNames}
          onChange={value => setTagNames(value as string[])}
          options={availableTagNames.map((name: string) => ({ label: name, value: name }))}
        />
      </div>

      {fileList.length > 0 ? (
        <div className="actions-area">
          <Button type="primary" onClick={upload} loading={loading}>
            开始上传 ({fileList.length})
          </Button>
          <Button onClick={() => setFileList([])} disabled={loading}>
            清空
          </Button>
        </div>
      ) : null}
    </div>
  )
}
