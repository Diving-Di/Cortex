import type { ChangeEvent } from 'react';
import { useState } from 'react';
import type { AxiosError } from 'axios';
import { Button, Card, Form, Input, Tabs, message } from 'antd';
import { EditOutlined, LockOutlined, MailOutlined, UserOutlined } from '@ant-design/icons';
import { loginUser, registerUser } from '../../api/auth';
import './Auth.css';

interface AuthProps {
  onLogin: (username: string) => void;
}

type APIErrorBody = { code?: string; message?: string; details?: unknown };

function apiErrorMessage(error: unknown, fallback: string) {
  const value = error as AxiosError<APIErrorBody>;
  return value.response?.data?.message || fallback;
}

export default function Auth({ onLogin }: AuthProps) {
  const [view, setView] = useState('login');
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [email, setEmail] = useState('');
  const [loading, setLoading] = useState(false);

  async function handleLogin() {
    if (!username || !password) {
      message.warning('请填写所有字段');
      return;
    }
    setLoading(true);
    try {
      const res = await loginUser({ username, password });
      onLogin(res.username);
      message.success('登录成功');
    } catch (e) {
      message.error(`登录失败：${apiErrorMessage(e, '用户名或密码错误')}`);
    } finally {
      setLoading(false);
    }
  }

  async function handleRegister() {
    if (!username || !password || !email) {
      message.warning('请填写所有字段');
      return;
    }
    setLoading(true);
    try {
      await registerUser({ username, password, email });
      message.success('注册成功，请登录');
      setView('login');
    } catch (e) {
      message.error(`注册失败：${apiErrorMessage(e, '请检查注册信息')}`);
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="auth-page">
      <div className="auth-brand">
        <EditOutlined className="auth-brand-icon" />
        <h1>Cortex</h1>
        <p>记录日常，整理思绪，找回重要的片段</p>
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
                  <Form.Item label="密码" extra="至少 12 个字符，避免使用常见密码">
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
  );
}
