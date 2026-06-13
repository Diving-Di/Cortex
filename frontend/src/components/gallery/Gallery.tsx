import { forwardRef, useEffect, useImperativeHandle, useState } from 'react'
import { type AxiosError } from 'axios'
import { Button, Card, Empty, Image, Modal, Select, Spin, Tag, message } from 'antd'
import { DeleteOutlined, PictureOutlined } from '@ant-design/icons'
import { deleteImage, fetchImages as fetchImageList, generateAiTags, updateImageTags } from '../../api/images'
import { fetchCustomTagNames } from '../../api/tags'
import type { ImageId, ImageItem } from '../../types/image'
import { formatDate } from '../../utils/date'
import { getAiTags, getCustomTags } from '../../utils/images'
import { getFullImageUrl } from '../../utils/url'
import './Gallery.css'

interface GalleryProps {
  token: string
  filterTag?: string
  onImagesChanged?: (images: ImageItem[]) => void
}

export interface GalleryHandle {
  fetchImages: () => Promise<void>
}

const Gallery = forwardRef<GalleryHandle, GalleryProps>(function Gallery({ token, filterTag = '', onImagesChanged }, ref) {
  const [images, setImages] = useState<ImageItem[]>([])
  const [loading, setLoading] = useState(false)
  const [availableTagNames, setAvailableTagNames] = useState<string[]>([])
  const [editTagNames, setEditTagNames] = useState<Record<ImageId, string[]>>({})
  const [saving, setSaving] = useState<Record<ImageId, boolean>>({})
  const [aiLoading, setAiLoading] = useState<Record<ImageId, boolean>>({})

  useImperativeHandle(ref, () => ({ fetchImages }))

  useEffect(() => {
    if (!token) {
      setImages([])
      onImagesChanged?.([])
      setAvailableTagNames([])
      setEditTagNames({})
      return
    }
    fetchTags()
    fetchImages()
  }, [token])

  useEffect(() => {
    if (token) {
      fetchImages()
    }
  }, [filterTag])

  async function fetchTags() {
    if (!token) return
    try {
      setAvailableTagNames(await fetchCustomTagNames(token))
    } catch (err) {
      console.error(err)
    }
  }

  async function fetchImages() {
    if (!token) return
    setLoading(true)
    try {
      const nextImages = await fetchImageList(token, filterTag)
      setImages(nextImages)
      onImagesChanged?.(nextImages)
      setEditTagNames(current => {
        const nextEdit = { ...current }
        for (const item of nextImages || []) {
          if (!item || !item.id) continue
          const currentTags = getCustomTags(item)
            .map(t => t.tag_name)
            .filter((name): name is string => Boolean(name))
          if (!Array.isArray(nextEdit[item.id])) {
            nextEdit[item.id] = currentTags
          }
        }
        return nextEdit
      })
    } catch (err) {
      console.error(err)
      message.error('获取图片失败')
    } finally {
      setLoading(false)
    }
  }

  async function saveTags(item: ImageItem) {
    if (!item || !item.id || !token) return

    const raw = editTagNames[item.id] || []
    const cleaned = raw
      .map(s => String(s).trim())
      .filter(Boolean)
      .filter(s => s.replace(/\s+/g, '') !== '[]')

    setSaving(current => ({ ...current, [item.id]: true }))
    try {
      await updateImageTags(token, item.id, cleaned)
      message.success('标签已更新')
      await Promise.all([fetchTags(), fetchImages()])
    } catch (err) {
      console.error(err)
      message.error('标签更新失败')
    } finally {
      setSaving(current => ({ ...current, [item.id]: false }))
    }
  }

  function remove(id: ImageId) {
    Modal.confirm({
      title: '警告',
      content: '你想要删除这张图片吗？',
      okText: '删除',
      cancelText: '取消',
      okButtonProps: { danger: true },
      async onOk() {
        try {
          await deleteImage(token, id)
          message.success('图片已删除')
          fetchImages()
        } catch (err) {
          console.error(err)
          message.error('图片删除失败')
        }
      },
    })
  }

  async function aiTag(item: ImageItem) {
    if (!item || !item.id || !token) return
    setAiLoading(current => ({ ...current, [item.id]: true }))
    try {
      await generateAiTags(token, item.id)
      message.success('AI标签已生成')
      await fetchImages()
    } catch (err) {
      console.error(err)
      const error = err as AxiosError<{ detail?: string }>
      const msg = error.response?.data?.detail || 'AI标签生成失败（可检查是否配置AI密钥）'
      message.error(msg)
    } finally {
      setAiLoading(current => ({ ...current, [item.id]: false }))
    }
  }

  return (
    <Spin spinning={loading}>
      {!loading && images.length === 0 ? (
        <Empty description="未找到图片" />
      ) : (
        <div className="gallery-grid">
          {images.map(item => (
            <Card
              key={item.id}
              styles={{ body: { padding: 0 } }}
              hoverable
              className="gallery-item"
            >
              <div className="image-wrapper">
                <Image
                  src={getFullImageUrl(item.thumbnail || item.image)}
                  preview={{ src: getFullImageUrl(item.image) }}
                  className="gallery-image"
                  alt={item.image?.split('/').pop() || `图片 ${item.id}`}
                  fallback=""
                  placeholder={
                    <div className="image-slot">
                      <PictureOutlined />
                    </div>
                  }
                />
              </div>
              <div className="card-content">
                <div className="card-left">
                  <div className="image-name" title={item.image?.split('/').pop()}>
                    {item.image?.split('/').pop()}
                  </div>

                  {(getCustomTags(item).length ||
                    getAiTags(item).length ||
                    item.exif_datetime ||
                    item.created ||
                    item.location ||
                    item.resolution) ? (
                    <div className="meta-area">
                      {getCustomTags(item).length ? (
                        <div className="tag-list">
                          {getCustomTags(item).map(t => (
                            <Tag key={t.id || t.tag_name} className="tag-item">
                              {t.tag_name}
                            </Tag>
                          ))}
                        </div>
                      ) : null}

                      {getAiTags(item).length ? (
                        <div className="tag-list">
                          {getAiTags(item).map(t => (
                            <Tag key={`ai-${t.id || t.tag_name}`} color="success" className="tag-item">
                              AI: {t.tag_name}
                            </Tag>
                          ))}
                        </div>
                      ) : null}

                      {item.exif_datetime ? (
                        <div className="meta-line">拍摄时间：{formatDate(item.exif_datetime)}</div>
                      ) : item.created ? (
                        <div className="meta-line">上传时间：{formatDate(item.created)}</div>
                      ) : null}
                      {item.location ? <div className="meta-line">地点：{item.location}</div> : null}
                      {item.resolution ? <div className="meta-line">分辨率：{item.resolution}</div> : null}

                      <div className="edit-tags">
                        <Select
                          mode="tags"
                          placeholder="为该图片添加自定义标签"
                          size="small"
                          className="edit-tag-select"
                          value={editTagNames[item.id] || []}
                          onChange={value => {
                            setEditTagNames(current => ({ ...current, [item.id]: value as string[] }))
                          }}
                          options={availableTagNames.map(name => ({ label: name, value: name }))}
                        />
                        <Button
                          size="small"
                          type="primary"
                          loading={saving[item.id]}
                          onClick={() => saveTags(item)}
                        >
                          保存
                        </Button>
                      </div>
                    </div>
                  ) : null}
                </div>

                <div className="card-actions">
                  <Button
                    type="primary"
                    shape="circle"
                    size="small"
                    loading={aiLoading[item.id]}
                    onClick={() => aiTag(item)}
                    title="AI生成标签"
                  >
                    AI
                  </Button>
                  <Button
                    danger
                    shape="circle"
                    size="small"
                    icon={<DeleteOutlined />}
                    onClick={() => remove(item.id)}
                  />
                </div>
              </div>
            </Card>
          ))}
        </div>
      )}
    </Spin>
  )
})

export default Gallery
