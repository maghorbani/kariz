import type { ReactNode } from 'react';
import { Navigate } from 'react-router-dom';
import { Result, Spin } from 'antd';
import { useAuth } from '@/context/AuthContext';
import { isAdmin } from '@/utils/auth';

interface AdminRouteProps {
  children: ReactNode;
}

export default function AdminRoute({ children }: AdminRouteProps) {
  const { user, loading } = useAuth();

  if (loading) {
    return (
      <div
        style={{
          display: 'flex',
          justifyContent: 'center',
          alignItems: 'center',
          minHeight: '100vh',
        }}
      >
        <Spin size="large" />
      </div>
    );
  }

  if (!user) {
    return <Navigate to="/login" replace />;
  }

  if (!isAdmin(user)) {
    return (
      <Result
        status="403"
        title="Access denied"
        subTitle="You need administrator privileges to view this page."
      />
    );
  }

  return <>{children}</>;
}
