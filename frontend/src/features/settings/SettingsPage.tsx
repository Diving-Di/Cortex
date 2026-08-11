import { useEffect, useState } from 'react';
import { Card, Segmented, Space, Spin, Switch, Typography, message } from 'antd';
import { ThemePreference, useTheme } from '../../app/theme';
import { getPreferences, updatePreferences, type Preferences } from '../../api/settings';
import './SettingsPage.css';

const themeOptions = [
  { label: '跟随系统', value: 'system' },
  { label: '浅色', value: 'light' },
  { label: '深色', value: 'dark' },
];

export default function SettingsPage() {
  const { preference, setPreference } = useTheme();
  const [preferences, setPreferences] = useState<Preferences | null>(null);
  const [savingPreferences, setSavingPreferences] = useState(false);

  useEffect(() => {
    void getPreferences()
      .then(setPreferences)
      .catch(() => message.error('无法加载设置'));
  }, []);

  return (
    <div className="settings-page">
      <div className="settings-heading">
        <Typography.Title level={1}>设置</Typography.Title>
        <Typography.Text type="secondary">管理页面外观和模板广场偏好。</Typography.Text>
      </div>

      <Card title="外观" className="settings-card">
        <div className="settings-row">
          <div>
            <Typography.Title level={5}>页面主题</Typography.Title>
            <Typography.Text type="secondary">
              主题仅保存在当前浏览器，不会上传服务端。
            </Typography.Text>
          </div>
          <Segmented
            aria-label="页面主题"
            options={themeOptions}
            value={preference}
            onChange={(value) => setPreference(value as ThemePreference)}
          />
        </div>
      </Card>

      <Card title="模板广场" className="settings-card">
        <Space>
          <Switch
            aria-label="个性化模板推荐"
            checked={preferences?.marketplace_personalization ?? true}
            disabled={!preferences || savingPreferences}
            onChange={(checked) => {
              if (!preferences) return;
              const next = { ...preferences, marketplace_personalization: checked };
              setSavingPreferences(true);
              void updatePreferences(next)
                .then(setPreferences)
                .catch(() => message.error('保存失败，设置可能已在其他设备更新'))
                .finally(() => setSavingPreferences(false));
            }}
          />
          <Typography.Text>根据收藏和使用记录提供个性化模板推荐</Typography.Text>
        </Space>
      </Card>
    </div>
  );
}
