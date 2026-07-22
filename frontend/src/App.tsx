import { lazy, Suspense } from 'react'
import { Button, Menu, Spin } from 'antd'
import { EditOutlined, LogoutOutlined, MessageOutlined, RobotOutlined } from '@ant-design/icons'
import { Navigate, Route, Routes, useLocation, useNavigate } from 'react-router-dom'
import ProtectedRoute from './routes/ProtectedRoute'
import './App.css'

const LoginPage = lazy(() => import('./features/auth/LoginPage'))
const DashboardPage = lazy(() => import('./features/dashboard/DashboardPage'))
const NotesPage = lazy(() => import('./features/notes/NotesPage'))
const ReportsPage = lazy(() => import('./features/reports/ReportsPage'))
const MemoryPage = lazy(() => import('./features/memory/MemoryPage'))
const SearchPage = lazy(() => import('./features/search/SearchPage'))
const SettingsPage = lazy(() => import('./features/settings/SettingsPage'))

function AppLayout() {
  const navigate = useNavigate()
  const location = useLocation()
  const token = localStorage.getItem('token') as string
  const username = localStorage.getItem('username') || ''

  function logout() {
    localStorage.removeItem('token')
    localStorage.removeItem('username')
    navigate('/login', { replace: true })
  }

  const selected = location.pathname.startsWith('/notes') ? '/notes' : location.pathname
  return (
    <div className="app">
      <nav className="app-nav">
        <div className="app-logo"><RobotOutlined className="app-logo-icon" /><span>Diary Listener</span></div>
        <Menu mode="inline" selectedKeys={[selected]} onClick={({ key }) => navigate(key)} className="app-menu" items={[
          { key: '/', icon: <MessageOutlined />, label: '工作台' },
          { key: '/notes', icon: <EditOutlined />, label: '笔记本' },
          { key: '/reports', label: '周期报告' },
          { key: '/memory', label: '回忆书' },
          { key: '/search', label: '搜索' },
          { key: '/settings', label: '设置' },
        ]} />
        <div className="app-nav-footer"><div className="app-user">{username}</div><Button type="text" icon={<LogoutOutlined />} onClick={logout} className="app-logout">退出登录</Button></div>
      </nav>
      <main className="app-content"><Suspense fallback={<Spin />}><Routes>
        <Route index element={<DashboardPage token={token} />} />
        <Route path="notes/*" element={<NotesPage token={token} />} />
        <Route path="reports" element={<ReportsPage />} />
        <Route path="memory" element={<MemoryPage />} />
        <Route path="search" element={<SearchPage />} />
        <Route path="settings" element={<SettingsPage />} />
      </Routes></Suspense></main>
    </div>
  )
}

export default function App() {
  return <Suspense fallback={<Spin fullscreen />}><Routes>
    <Route path="/login" element={<LoginPage />} />
    <Route element={<ProtectedRoute />}><Route path="/*" element={<AppLayout />} /></Route>
    <Route path="*" element={<Navigate to="/" replace />} />
  </Routes></Suspense>
}
