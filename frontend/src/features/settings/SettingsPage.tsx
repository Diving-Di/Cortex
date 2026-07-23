import { Card, Segmented, Typography } from 'antd';
import { ThemePreference, useTheme } from '../../app/theme';
import './SettingsPage.css';

const themeOptions = [
  { label: '跟随系统', value: 'system' },
  { label: '浅色', value: 'light' },
  { label: '深色', value: 'dark' },
];

export default function SettingsPage() {
  const { preference, setPreference } = useTheme();

  return (
    <div className="settings-page">
      <div className="settings-heading">
        <Typography.Title level={1}>设置</Typography.Title>
        <Typography.Text type="secondary">管理当前浏览器的外观偏好。</Typography.Text>
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
    </div>
  );
}
