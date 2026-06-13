import { useState } from 'react'
import { Button, Menu } from 'antd'
import {
  EditOutlined,
  LogoutOutlined,
  MessageOutlined,
  RobotOutlined,
} from '@ant-design/icons'
import Auth from './components/auth/Auth'
import Chat from './components/chat/Chat'
import Diary from './components/diary/Diary'
import './App.css'

type FeatureKey = 'chat' | 'diary'

export default function App() {
  const [token, setToken] = useState<string | null>(() => localStorage.getItem('token'))
  const [username, setUsername] = useState<string>(() => localStorage.getItem('username') || '')
  const [feature, setFeature] = useState<FeatureKey>('chat')

  function handleLogin(nextToken: string, nextUsername: string) {
    setToken(nextToken)
    setUsername(nextUsername)
    localStorage.setItem('token', nextToken)
    localStorage.setItem('username', nextUsername)
  }

  function handleLogout() {
    setToken(null)
    setUsername('')
    localStorage.removeItem('token')
    localStorage.removeItem('username')
  }

  if (!token) {
    return <Auth onLogin={handleLogin} />
  }

  return (
    <div className="app">
      <nav className="app-nav">
        <div className="app-logo">
          <RobotOutlined className="app-logo-icon" />
          <span>AI 助手</span>
        </div>

        <Menu
          mode="inline"
          selectedKeys={[feature]}
          onClick={(e) => setFeature(e.key as FeatureKey)}
          className="app-menu"
          items={[
            { key: 'chat', icon: <MessageOutlined />, label: '聊天' },
            { key: 'diary', icon: <EditOutlined />, label: '日记' },
          ]}
        />

        <div className="app-nav-footer">
          <div className="app-user">{username}</div>
          <Button
            type="text"
            icon={<LogoutOutlined />}
            onClick={handleLogout}
            className="app-logout"
          >
            退出登录
          </Button>
        </div>
      </nav>

      <main className="app-content">
        {feature === 'chat' ? <Chat token={token} /> : <Diary token={token} />}
      </main>
    </div>
  )
}
