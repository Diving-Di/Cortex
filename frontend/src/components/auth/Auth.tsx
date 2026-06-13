import type { ChangeEvent } from 'react'
import { useState } from 'react'
import type { AxiosError } from 'axios'
import { Button, Card, Form, Input, Tabs, message } from 'antd'
import { LockOutlined, MailOutlined, MessageOutlined, UserOutlined } from '@ant-design/icons'
import { loginUser, registerUser } from '../../api/auth'
import './Auth.css'

interface AuthProps {
  onLogin: (token: string, username: string) => void
}

export default function Auth({ onLogin }: AuthProps) {
  const [view, setView] = useState('login')
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [email, setEmail] = useState('')
  const [loading, setLoading] = useState(false)

  async function handleLogin() {
    if (!username || !password) {
      message.warning('请填写所有字段')
      return
    }
    setLoading(true)
    try {
      const res = await loginUser({ username, password })
      onLogin(res.token, res.username)
      message.success('登录成功')
    } catch (e) {
      const err = e as AxiosError<{ detail?: string }>
      message.error(`登录失败：${err.response?.data?.detail || '用户名或密码错误'}`)
    } finally {
      setLoading(false)
    }
  }

  async function handleRegister() {
    if (!username || !password || !email) {
      message.warning('请填写所有字段')
      return
    }
    setLoading(true)
    try {
      await registerUser({ username, password, email })
      message.success('注册成功，请登录')
      setView('login')
    } catch (e) {
      const err = e as AxiosError<{ detail?: string }>
      message.error(`注册失败：${err.response?.data?.detail || err.message}`)
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="auth-page">
      <div className="auth-brand">
        <MessageOutlined className="auth-brand-icon" />
        <h1>AI 聊天 & 轻日记</h1>
        <p>与 AI 多轮对话，用一张图 + 一段话记录生活</p>
      </div>
      <Card className="auth-card">
        <Tabs
          activeKey={view}
          centered
          onChange={setView}
          items={[
            {
              key: 'login',
              label: '登录',
              children: (
                <Form layout="vertical" onFinish={handleLogin}>
                  <Form.Item label="用户名">
                    <Input
                      value={username}
                      onChange={(e: ChangeEvent<HTMLInputElement>) => setUsername(e.target.value)}
                      placeholder="请输入用户名"
                      prefix={<UserOutlined />}
                    />
                  </Form.Item>
                  <Form.Item label="密码">
                    <Input.Password
                      value={password}
                      onChange={(e: ChangeEvent<HTMLInputElement>) => setPassword(e.target.value)}
                      placeholder="请输入密码"
                      prefix={<LockOutlined />}
                    />
                  </Form.Item>
                  <Button type="primary" htmlType="submit" block loading={loading}>
                    登录
                  </Button>
                </Form>
              ),
            },
            {
              key: 'register',
              label: '注册',
              children: (
                <Form layout="vertical" onFinish={handleRegister}>
                  <Form.Item label="用户名" extra="至少 6 个字符">
                    <Input
                      value={username}
                      onChange={(e: ChangeEvent<HTMLInputElement>) => setUsername(e.target.value)}
                      placeholder="请输入用户名"
                      prefix={<UserOutlined />}
                    />
                  </Form.Item>
                  <Form.Item label="邮箱">
                    <Input
                      value={email}
                      onChange={(e: ChangeEvent<HTMLInputElement>) => setEmail(e.target.value)}
                      placeholder="请输入邮箱"
                      prefix={<MailOutlined />}
                    />
                  </Form.Item>
                  <Form.Item label="密码" extra="至少 6 个字符">
                    <Input.Password
                      value={password}
                      onChange={(e: ChangeEvent<HTMLInputElement>) => setPassword(e.target.value)}
                      placeholder="请输入密码"
                      prefix={<LockOutlined />}
                    />
                  </Form.Item>
                  <Button type="primary" htmlType="submit" block loading={loading}>
                    注册
                  </Button>
                </Form>
              ),
            },
          ]}
        />
      </Card>
    </div>
  )
}
