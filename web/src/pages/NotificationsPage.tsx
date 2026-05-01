import { useCallback, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import {
  Alert,
  Badge,
  List,
  Radio,
  Space,
  Spin,
  Typography,
} from 'antd';
import {
  BellOutlined,
  CheckCircleOutlined,
  CloseCircleOutlined,
  ExclamationCircleOutlined,
} from '@ant-design/icons';
import { get, put } from '@/api/client';
import type { Notification, NotificationType } from '@/types';

const { Title, Text, Paragraph } = Typography;

type FilterMode = 'all' | 'unread';

const NOTIFICATION_ICON: Record<NotificationType, React.ReactNode> = {
  execution_complete: (
    <CheckCircleOutlined style={{ color: '#52c41a', fontSize: 20 }} />
  ),
  execution_failed: (
    <CloseCircleOutlined style={{ color: '#ff4d4f', fontSize: 20 }} />
  ),
  execution_timed_out: (
    <ExclamationCircleOutlined style={{ color: '#faad14', fontSize: 20 }} />
  ),
};

function formatTime(iso: string): string {
  const date = new Date(iso);
  const now = new Date();
  const diffMs = now.getTime() - date.getTime();
  const diffMin = Math.floor(diffMs / 60000);
  if (diffMin < 1) return 'Just now';
  if (diffMin < 60) return `${diffMin}m ago`;
  const diffHours = Math.floor(diffMin / 60);
  if (diffHours < 24) return `${diffHours}h ago`;
  const diffDays = Math.floor(diffHours / 24);
  if (diffDays < 7) return `${diffDays}d ago`;
  return date.toLocaleDateString();
}

export default function NotificationsPage() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [filterMode, setFilterMode] = useState<FilterMode>('all');

  const {
    data: notifications,
    isLoading,
    error,
  } = useQuery<Notification[]>({
    queryKey: ['notifications', filterMode],
    queryFn: () => {
      const params =
        filterMode === 'unread' ? '?unread_only=true' : '';
      return get<Notification[]>(`/notifications${params}`);
    },
  });

  const markReadMutation = useMutation({
    mutationFn: (notificationId: string) =>
      put(`/notifications/${notificationId}/read`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['notifications'] });
    },
  });

  const handleNotificationClick = useCallback(
    (notification: Notification) => {
      if (!notification.is_read) {
        markReadMutation.mutate(notification.id);
      }
      if (notification.execution_id) {
        navigate(`/executions/${notification.execution_id}`);
      }
    },
    [markReadMutation, navigate],
  );

  const unreadCount =
    notifications?.filter((n) => !n.is_read).length ?? 0;

  return (
    <div style={{ padding: 24, maxWidth: 800, margin: '0 auto' }}>
      <Space
        style={{
          display: 'flex',
          justifyContent: 'space-between',
          marginBottom: 16,
        }}
      >
        <Space align="center">
          <Title level={2} style={{ margin: 0 }}>
            Notifications
          </Title>
          {unreadCount > 0 && (
            <Badge count={unreadCount} style={{ marginLeft: 8 }} />
          )}
        </Space>
        <Radio.Group
          value={filterMode}
          onChange={(e) => setFilterMode(e.target.value)}
          optionType="button"
          buttonStyle="solid"
        >
          <Radio.Button value="all">All</Radio.Button>
          <Radio.Button value="unread">Unread</Radio.Button>
        </Radio.Group>
      </Space>

      {error && (
        <Alert
          message="Failed to load notifications"
          description={
            error instanceof Error
              ? error.message
              : 'An unexpected error occurred.'
          }
          type="error"
          showIcon
          style={{ marginBottom: 16 }}
        />
      )}

      {isLoading && (
        <div
          style={{
            display: 'flex',
            justifyContent: 'center',
            padding: 64,
          }}
        >
          <Spin size="large" />
        </div>
      )}

      {!isLoading && !error && notifications && notifications.length === 0 && (
        <div
          style={{
            textAlign: 'center',
            padding: 64,
            color: '#999',
          }}
        >
          <BellOutlined style={{ fontSize: 48, marginBottom: 16 }} />
          <Paragraph type="secondary">
            {filterMode === 'unread'
              ? 'No unread notifications'
              : 'No notifications yet'}
          </Paragraph>
        </div>
      )}

      {!isLoading && !error && notifications && notifications.length > 0 && (
        <List
          dataSource={notifications}
          renderItem={(notification) => (
            <List.Item
              onClick={() => handleNotificationClick(notification)}
              style={{
                cursor: 'pointer',
                background: notification.is_read
                  ? 'transparent'
                  : '#e6f4ff',
                padding: '12px 16px',
                borderRadius: 6,
                marginBottom: 4,
              }}
              extra={
                <Text type="secondary" style={{ fontSize: 12, whiteSpace: 'nowrap' }}>
                  {formatTime(notification.created_at)}
                </Text>
              }
            >
              <List.Item.Meta
                avatar={
                  NOTIFICATION_ICON[notification.type] ?? (
                    <BellOutlined style={{ fontSize: 20 }} />
                  )
                }
                title={
                  <Space>
                    <Text strong={!notification.is_read}>
                      {notification.title}
                    </Text>
                    {!notification.is_read && (
                      <Badge status="processing" />
                    )}
                  </Space>
                }
                description={notification.message}
              />
            </List.Item>
          )}
        />
      )}
    </div>
  );
}
