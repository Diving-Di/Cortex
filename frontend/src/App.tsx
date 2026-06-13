import type { ChangeEvent } from 'react'
import { useEffect, useMemo, useRef, useState } from 'react'
import { type AxiosError } from 'axios'
import {
  Button,
  Card,
  Form,
  Input,
  Layout,
  Select,
  Space,
  Tabs,
  message,
} from 'antd'
import { LockOutlined, MailOutlined, PictureOutlined, UserOutlined } from '@ant-design/icons'
import ImageCarousel from './components/carousel/ImageCarousel'
import Gallery, { type GalleryHandle } from './components/gallery/Gallery'
import ImageUpload from './components/upload/ImageUpload'
import { loginUser, registerUser } from './api/auth'
import { fetchCustomTagNames } from './api/tags'
import type { ImageId, ImageItem } from './types/image'
import './App.css'

const { Header, Content } = Layout

function loadCarouselSelectedIdsSafe(): ImageId[] {
  try {
    const raw = localStorage.getItem('carousel_selected_ids')
    const parsed = raw ? JSON.parse(raw) : []
    return Array.isArray(parsed) ? parsed : []
  } catch (e) {
    return []
  }
}

export default function App() {
  const [view, setView] = useState('login')
  const [username, setUsername] = useState('')
  const [displayUsername, setDisplayUsername] = useState(localStorage.getItem('username') || '')
  const [password, setPassword] = useState('')
  const [email, setEmail] = useState('')
  const [token, setToken] = useState(localStorage.getItem('token') || null)
  const [loading, setLoading] = useState(false)
  const [filterTag, setFilterTag] = useState('')
  const [availableTagNames, setAvailableTagNames] = useState<string[]>([])
  const [imagesForCarousel, setImagesForCarousel] = useState<ImageItem[]>([])
  const [carouselSelectedIds, setCarouselSelectedIds] = useState<ImageId[]>(loadCarouselSelectedIdsSafe)
  const [editingCarousel, setEditingCarousel] = useState(false)
  const [carouselDraftIds, setCarouselDraftIds] = useState<ImageId[]>([])
  const galleryRef = useRef<GalleryHandle>(null)

  const carouselItems = useMemo(() => {
    const selected = new Set(carouselSelectedIds || [])
    return (imagesForCarousel || []).filter(img => selected.has(img.id))
  }, [carouselSelectedIds, imagesForCarousel])

  useEffect(() => {
    if (token) {
      fetchTags()
      return
    }
    setFilterTag('')
    setAvailableTagNames([])
    setImagesForCarousel([])
    setCarouselSelectedIds([])
    setEditingCarousel(false)
    setCarouselDraftIds([])
    localStorage.removeItem('carousel_selected_ids')
  }, [token])

  useEffect(() => {
    localStorage.setItem('carousel_selected_ids', JSON.stringify(carouselSelectedIds || []))
  }, [carouselSelectedIds])

  async function fetchTags() {
    if (!token) return
    try {
      setAvailableTagNames(await fetchCustomTagNames(token))
    } catch (e) {
      console.error(e)
    }
  }

  function applyFilter() {
    galleryRef.current?.fetchImages?.()
  }

  function onImagesChanged(images: ImageItem[]) {
    const nextImages = Array.isArray(images) ? images : []
    setImagesForCarousel(nextImages)
    const valid = new Set(nextImages.map(x => x && x.id).filter(Boolean))
    setCarouselSelectedIds((prev: ImageId[]) => (prev || []).filter((id: ImageId) => valid.has(id)))
    setCarouselDraftIds((prev: ImageId[]) => (prev || []).filter((id: ImageId) => valid.has(id)))
  }

  function toggleCarouselEdit() {
    if (!editingCarousel) {
      setCarouselDraftIds([...(carouselSelectedIds || [])])
      setEditingCarousel(true)
      return
    }
    setCarouselDraftIds([])
    setEditingCarousel(false)
  }


  function saveCarouselEdit() {
    setCarouselSelectedIds([...(carouselDraftIds || [])])
    setEditingCarousel(false)
    setCarouselDraftIds([])
  }

  async function login() {
    if (!username || !password) {
      message.warning('请填写所有字段')
      return
    }
    setLoading(true)
    try {
      const res = await loginUser({ username, password })
      setToken(res.token)
      localStorage.setItem('token', res.token)
      setDisplayUsername(username)
      localStorage.setItem('username', username)
      message.success('登录成功')
    } catch (e) {
      console.error(e)
      const err = e as AxiosError
      if (err.response) {
        message.error('登录失败：用户名或密码错误')
      } else if (err.request) {
        message.error('登录失败：无法连接到服务器，请检查网络或防火墙')
      } else {
        message.error(`登录失败：${err.message}`)
      }
    } finally {
      setLoading(false)
    }
  }

  async function register() {
    if (!username || !password || !email) {
      message.warning('请填写所有字段')
      return
    }
    setLoading(true)
    try {
      await registerUser({ username, password, email })
      message.success('注册成功，请登录。')
      setView('login')
    } catch (e) {
      console.error(e)
      const err = e as AxiosError
      if (err.response && err.response.data) {
        message.error(`注册失败：${JSON.stringify(err.response.data)}`)
      } else {
        message.error(`注册失败：${err.message}`)
      }
    } finally {
      setLoading(false)
    }
  }

  function logout() {
    setToken(null)
    localStorage.removeItem('token')
    localStorage.removeItem('username')
    setUsername('')
    setDisplayUsername('')
    setPassword('')
    message.info('退出登录成功')
  }

  function reload() {
    galleryRef.current?.fetchImages?.()
    fetchTags()
  }

  return (
    <Layout className="app-container">
      <Header className="app-header">
        <div className="header-content">
          <div className="logo">
            <PictureOutlined className="logo-icon" />
            <span>Image Manager</span>
          </div>
          {token ? (
            <div className="user-info">
              <span className="welcome-text">欢迎登录, {displayUsername}</span>
              <Button danger size="small" onClick={logout}>
                退出登录
              </Button>
            </div>
          ) : null}
        </div>
      </Header>

      <Content className="container">
        {!token ? (
          <div className="auth-container">
            <Card className="auth-card">
              <Tabs
                activeKey={view}
                centered
                onChange={setView}
                items={[
                  {
                    key: 'login',
                    label: 'Login',
                    children: (
                      <Form layout="vertical" onFinish={login}>
                        <Form.Item label="Username">
                          <Input
                            value={username}
                            onChange={(e: ChangeEvent<HTMLInputElement>) => setUsername(e.target.value)}
                            placeholder="请输入用户名"
                            prefix={<UserOutlined />}
                          />
                        </Form.Item>
                        <Form.Item label="Password">
                          <Input.Password
                            value={password}
                            onChange={(e: ChangeEvent<HTMLInputElement>) => setPassword(e.target.value)}
                            placeholder="请输入密码"
                            prefix={<LockOutlined />}
                          />
                        </Form.Item>
                        <Button type="primary" htmlType="submit" className="w-100" loading={loading}>
                          登录
                        </Button>
                      </Form>
                    ),
                  },
                  {
                    key: 'register',
                    label: 'Register',
                    children: (
                      <Form layout="vertical" onFinish={register}>
                        <Form.Item label="Username">
                          <Input
                            value={username}
                            onChange={(e: ChangeEvent<HTMLInputElement>) => setUsername(e.target.value)}
                            placeholder="请输入用户名"
                            prefix={<UserOutlined />}
                          />
                        </Form.Item>
                        <Form.Item label="Email">
                          <Input
                            value={email}
                            onChange={(e: ChangeEvent<HTMLInputElement>) => setEmail(e.target.value)}
                            placeholder="请输入邮箱"
                            prefix={<MailOutlined />}
                          />
                        </Form.Item>
                        <Form.Item label="Password">
                          <Input.Password
                            value={password}
                            onChange={(e: ChangeEvent<HTMLInputElement>) => setPassword(e.target.value)}
                            placeholder="请输入密码"
                            prefix={<LockOutlined />}
                          />
                        </Form.Item>
                        <Button type="primary" htmlType="submit" className="w-100" loading={loading}>
                          注册
                        </Button>
                      </Form>
                    ),
                  },
                ]}
              />
            </Card>
          </div>
        ) : (
          <>
            <div className="top-row mb-20">
              <div className="upload-col">
                <Card title={<span className="card-header">上传新图片</span>}>
                  <ImageUpload token={token} onUploaded={reload} />
                </Card>
              </div>

              <div className="carousel-col">
                <Card>
                  <div className="carousel-topbar">
                    <Space>
                      <Button type="primary" ghost onClick={toggleCarouselEdit}>
                        {editingCarousel ? '取消编辑' : '编辑轮播'}
                      </Button>
                      {editingCarousel ? (
                        <Button type="primary" onClick={saveCarouselEdit}>
                          保存
                        </Button>
                      ) : null}
                    </Space>
                  </div>

                  {editingCarousel ? (
                    <div className="carousel-editor">
                      <div className="control-label">选择轮播图片（多选）</div>
                      <Select
                        mode="multiple"
                        allowClear
                        showSearch
                        placeholder="选择需要轮播的图片"
                        className="control-select"
                        value={carouselDraftIds}
                        onChange={value => setCarouselDraftIds(value as ImageId[])}
                        options={(imagesForCarousel || []).map(img => ({
                          label: `图片 ${img.id}`,
                          value: img.id,
                        }))}
                      />
                    </div>
                  ) : null}

                  <ImageCarousel items={carouselItems} />

                  <div className="search-area">
                    <div className="control-label">按标签检索</div>
                    <div className="search-row">
                      <Select
                        allowClear
                        showSearch
                        mode="tags"
                        maxCount={1}
                        placeholder="输入后回车"
                        className="control-select"
                        value={filterTag ? [filterTag] : []}
                        onChange={values => {
                          const selectedTags = values as string[]
                          const next = selectedTags?.[selectedTags.length - 1] || ''
                          setFilterTag(next)
                          window.setTimeout(applyFilter, 0)
                        }}
                        options={availableTagNames.map(name => ({ label: name, value: name }))}
                      />
                      <Button className="search-btn" onClick={applyFilter}>
                        检索
                      </Button>
                    </div>
                  </div>
                </Card>
              </div>
            </div>

            <Gallery
              ref={galleryRef}
              token={token}
              filterTag={filterTag}
              onImagesChanged={onImagesChanged}
            />
          </>
        )}
      </Content>
    </Layout>
  )
}
