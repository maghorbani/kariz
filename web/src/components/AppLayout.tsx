import type { ReactNode } from 'react';
import { Link, useLocation } from 'react-router-dom';
import {
  AppstoreOutlined,
  BellOutlined,
  CalendarOutlined,
  ControlOutlined,
  HistoryOutlined,
  LogoutOutlined,
  SettingOutlined,
  TeamOutlined,
  UserOutlined,
} from '@ant-design/icons';
import { Button, Layout, Menu, Space, Tag, Typography } from 'antd';
import { useAuth } from '@/context/AuthContext';
import { isAdmin, ROLE_COLORS } from '@/utils/auth';

const { Header, Sider, Content } = Layout;
const { Text } = Typography;

interface AppLayoutProps {
  children: ReactNode;
}

export default function AppLayout({ children }: AppLayoutProps) {
  const { user, logout } = useAuth();
  const location = useLocation();
  const admin = isAdmin(user);

  const selectedKey = (() => {
    const path = location.pathname;
    if (path.startsWith('/admin/users')) return '/admin/users';
    if (path.startsWith('/admin/commands')) return '/admin/commands';
    if (path.startsWith('/admin')) return '/admin';
    if (path.startsWith('/executions')) return '/executions';
    if (path.startsWith('/schedules')) return '/schedules';
    if (path.startsWith('/notifications')) return '/notifications';
    return '/';
  })();

  const menuItems = [
    {
      key: '/',
      icon: <AppstoreOutlined />,
      label: <Link to="/">Commands</Link>,
    },
    {
      key: '/executions',
      icon: <HistoryOutlined />,
      label: <Link to="/executions">Executions</Link>,
    },
    {
      key: '/schedules',
      icon: <CalendarOutlined />,
      label: <Link to="/schedules">Schedules</Link>,
    },
    {
      key: '/notifications',
      icon: <BellOutlined />,
      label: <Link to="/notifications">Notifications</Link>,
    },
    ...(admin
      ? [
          { type: 'divider' as const },
          {
            key: 'admin-group',
            label: 'Admin',
            type: 'group' as const,
            children: [
              {
                key: '/admin',
                icon: <ControlOutlined />,
                label: <Link to="/admin">Admin Panel</Link>,
              },
              {
                key: '/admin/commands',
                icon: <SettingOutlined />,
                label: <Link to="/admin/commands">Manage Commands</Link>,
              },
              {
                key: '/admin/users',
                icon: <TeamOutlined />,
                label: <Link to="/admin/users">Manage Users</Link>,
              },
            ],
          },
        ]
      : []),
  ];

  return (
    <Layout style={{ minHeight: '100vh' }}>
      <Sider
        breakpoint="lg"
        collapsedWidth={0}
        style={{ background: '#001529' }}
      >
        <div
          style={{
            height: 64,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            color: '#fff',
            fontWeight: 700,
            fontSize: 18,
            letterSpacing: 1,
          }}
        >
          KARIZ
        </div>
        <Menu
          theme="dark"
          mode="inline"
          selectedKeys={[selectedKey]}
          items={menuItems}
        />
      </Sider>
      <Layout>
        <Header
          style={{
            background: '#fff',
            padding: '0 24px',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            borderBottom: '1px solid #f0f0f0',
          }}
        >
          <Text type="secondary">Command Dashboard</Text>
          <Space>
            <UserOutlined />
            <Text strong>{user?.username}</Text>
            <Space size={[4, 4]}>
              {user?.roles.map((role) => (
                <Tag key={role} color={ROLE_COLORS[role] ?? 'default'}>
                  {role}
                </Tag>
              ))}
            </Space>
            <Button icon={<LogoutOutlined />} onClick={() => logout()}>
              Logout
            </Button>
          </Space>
        </Header>
        <Content style={{ padding: 24, background: '#f5f5f5' }}>
          {children}
        </Content>
      </Layout>
    </Layout>
  );
}
