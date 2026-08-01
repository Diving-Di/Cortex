import { useEffect, useState } from 'react';
import { Button, Card, Input, Segmented, Space, Spin, Switch, Typography, message } from 'antd';
import { ThemePreference, useTheme } from '../../app/theme';
import {
  getRecipePreferences,
  updateRecipePreferences,
  type RecipePreferences,
} from '../../api/recipes';
import './SettingsPage.css';

const themeOptions = [
  { label: '跟随系统', value: 'system' },
  { label: '浅色', value: 'light' },
  { label: '深色', value: 'dark' },
];

export default function SettingsPage({ token }: { token: string }) {
  const { preference, setPreference } = useTheme();
  const [preferences, setPreferences] = useState<RecipePreferences | null>(null);
  const [savingPreferences, setSavingPreferences] = useState(false);

  useEffect(() => {
    void getRecipePreferences(token)
      .then(setPreferences)
      .catch(() => message.error('无法加载忌口设置'));
  }, [token]);

  return (
    <div className="settings-page">
      <div className="settings-heading">
        <Typography.Title level={1}>设置</Typography.Title>
        <Typography.Text type="secondary">管理页面外观和个人饮食偏好。</Typography.Text>
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

      <Card title="饮食偏好" className="settings-card">
        <Typography.Title level={5}>忌口食材</Typography.Title>
        <Typography.Paragraph type="secondary">
          保存后会避开相关今日菜谱，并作为菜谱问答的系统约束，禁止推荐或要求使用这些食材。
        </Typography.Paragraph>
        {preferences ? (
          <Space.Compact style={{ width: '100%' }}>
            <Input
              aria-label="忌口食材"
              value={preferences.dietary_restrictions.join('，')}
              onChange={(event) =>
                setPreferences((current) =>
                  current
                    ? {
                        ...current,
                        dietary_restrictions: event.target.value
                          .split(/[，,]/)
                          .map((item) => item.trim())
                          .filter(Boolean),
                      }
                    : current,
                )
              }
              placeholder="例如：花生，香菜"
            />
            <Button
              loading={savingPreferences}
              onClick={() => {
                setSavingPreferences(true);
                void updateRecipePreferences(token, preferences)
                  .then((saved) => {
                    setPreferences(saved);
                    message.success('忌口设置已保存');
                  })
                  .catch(() => message.error('保存失败，设置可能已在其他设备更新'))
                  .finally(() => setSavingPreferences(false));
              }}
            >
              保存
            </Button>
          </Space.Compact>
        ) : (
          <Spin aria-label="正在加载忌口设置" />
        )}
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
              void updateRecipePreferences(token, next)
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
