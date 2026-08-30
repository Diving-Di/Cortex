import { lazy, Suspense } from 'react';
import { Button, Menu, Spin } from 'antd';
import {
  BarChartOutlined,
  BookOutlined,
  EditOutlined,
  LogoutOutlined,
  MessageOutlined,
  ThunderboltOutlined,
  SettingOutlined,
} from '@ant-design/icons';
import {
  Navigate,
  Route,
  Routes,
  useLocation,
  useNavigate,
  useOutletContext,
} from 'react-router-dom';
import ProtectedRoute, { type AuthenticatedOutletContext } from './routes/ProtectedRoute';
import { logoutUser } from './api/auth';
import './App.css';

const LoginPage = lazy(() => import('./features/auth/LoginPage'));
const DashboardPage = lazy(() => import('./features/dashboard/DashboardPage'));
const NotesPage = lazy(() => import('./features/notes/NotesPage'));
const ReportsPage = lazy(() => import('./features/reports/ReportsPage'));
const SettingsPage = lazy(() => import('./features/settings/SettingsPage'));
const KnowledgePage = lazy(() => import('./features/knowledge/KnowledgePage'));
const AIEventsPage = lazy(() => import('./features/aiEvents/AIEventsPage'));

function AppLayout() {
  const navigate = useNavigate();
  const location = useLocation();
  const { session } = useOutletContext<AuthenticatedOutletContext>();
  const username = session.username;

  async function logout() {
    try {
      await logoutUser();
    } finally {
      navigate('/login', { replace: true });
    }
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
            { key: '/knowledge', icon: <BookOutlined />, label: '个人知识库' },
            { key: '/ai-events', icon: <ThunderboltOutlined />, label: 'AI 限量活动' },
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
            <Route index element={<DashboardPage />} />
            <Route path="notes/*" element={<NotesPage />} />
            <Route path="knowledge" element={<KnowledgePage />} />
            <Route path="recipes" element={<Navigate to="/knowledge" replace />} />
            <Route path="assistant" element={<Navigate to="/knowledge" replace />} />
            <Route path="ai-events" element={<AIEventsPage />} />
            <Route path="reports" element={<ReportsPage />} />
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
