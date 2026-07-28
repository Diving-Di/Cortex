import { lazy, Suspense } from 'react';
import { Button, Menu, Spin } from 'antd';
import {
  BarChartOutlined,
  BookOutlined,
  BulbOutlined,
  HeartOutlined,
  EditOutlined,
  LogoutOutlined,
  MessageOutlined,
  SearchOutlined,
  SettingOutlined,
} from '@ant-design/icons';
import { Navigate, Route, Routes, useLocation, useNavigate } from 'react-router-dom';
import ProtectedRoute from './routes/ProtectedRoute';
import './App.css';

const LoginPage = lazy(() => import('./features/auth/LoginPage'));
const DashboardPage = lazy(() => import('./features/dashboard/DashboardPage'));
const NotesPage = lazy(() => import('./features/notes/NotesPage'));
const ReportsPage = lazy(() => import('./features/reports/ReportsPage'));
const SettingsPage = lazy(() => import('./features/settings/SettingsPage'));
const KnowledgePage = lazy(() => import('./features/knowledge/KnowledgePage'));
const GrowthAssistantPage = lazy(() => import('./features/assistant/GrowthAssistantPage'));
const ResearchPage = lazy(() => import('./features/research/ResearchPage'));
const MemoriesPage = lazy(() => import('./features/memories/MemoriesPage'));

function AppLayout() {
  const navigate = useNavigate();
  const location = useLocation();
  const token = localStorage.getItem('token') as string;
  const username = localStorage.getItem('username') || '';

  function logout() {
    localStorage.removeItem('token');
    localStorage.removeItem('username');
    navigate('/login', { replace: true });
  }

  const selected = location.pathname.startsWith('/notes') ? '/notes' : location.pathname;
  return (
    <div className="app">
      <nav className="app-nav">
        <div className="app-logo">
          <img className="app-logo-mark" src="/icons/app-icon.svg" alt="" />
          <span>Cortex</span>
        </div>
        <Menu
          mode="inline"
          selectedKeys={[selected]}
          onClick={({ key }) => navigate(key)}
          className="app-menu"
          items={[
            { key: '/', icon: <MessageOutlined />, label: '工作台' },
            { key: '/notes', icon: <EditOutlined />, label: '笔记本' },
            { key: '/knowledge', icon: <BookOutlined />, label: '知识库' },
            { key: '/assistant', icon: <BulbOutlined />, label: '成长助手' },
            { key: '/memories', icon: <HeartOutlined />, label: '成长记忆' },
            { key: '/research', icon: <SearchOutlined />, label: '小红书研究' },
            { key: '/reports', icon: <BarChartOutlined />, label: '周期报告' },
            { key: '/settings', icon: <SettingOutlined />, label: '设置' },
          ]}
        />
        <div className="app-nav-footer">
          <div className="app-user">{username}</div>
          <Button type="text" icon={<LogoutOutlined />} onClick={logout} className="app-logout">
            退出登录
          </Button>
        </div>
      </nav>
      <main className="app-content">
        <Suspense fallback={<Spin />}>
          <Routes>
            <Route index element={<DashboardPage token={token} />} />
            <Route path="notes/*" element={<NotesPage token={token} />} />
            <Route path="knowledge" element={<KnowledgePage token={token} />} />
            <Route path="assistant" element={<GrowthAssistantPage token={token} />} />
            <Route path="memories" element={<MemoriesPage token={token} />} />
            <Route path="research" element={<ResearchPage token={token} />} />
            <Route path="reports" element={<ReportsPage token={token} />} />
            <Route path="settings" element={<SettingsPage />} />
            <Route path="*" element={<Navigate to="/" replace />} />
          </Routes>
        </Suspense>
      </main>
    </div>
  );
}

export default function App() {
  return (
    <Suspense fallback={<Spin fullscreen />}>
      <Routes>
        <Route path="/login" element={<LoginPage />} />
        <Route element={<ProtectedRoute />}>
          <Route path="/*" element={<AppLayout />} />
        </Route>
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </Suspense>
  );
}
