import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { BrowserRouter, Outlet, Route, Routes } from 'react-router-dom';

import { AuthProvider } from './context/AuthContext';
import ProtectedRoute from './components/ProtectedRoute';
import AdminRoute from './components/AdminRoute';
import AppLayout from './components/AppLayout';

import LoginPage from './pages/LoginPage';
import CommandListPage from './pages/CommandListPage';
import CommandDetailPage from './pages/CommandDetailPage';
import ExecutionFormPage from './pages/ExecutionFormPage';
import ExecutionHistoryPage from './pages/ExecutionHistoryPage';
import ExecutionDetailPage from './pages/ExecutionDetailPage';
import NotificationsPage from './pages/NotificationsPage';
import AdminPanelPage from './pages/AdminPanelPage';
import CommandManagementPage from './pages/CommandManagementPage';
import UserManagementPage from './pages/UserManagementPage';
import ScheduleListPage from './pages/ScheduleListPage';

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 1,
      refetchOnWindowFocus: false,
      staleTime: 30_000,
    },
  },
});

function AuthenticatedLayout() {
  return (
    <ProtectedRoute>
      <AppLayout>
        <Outlet />
      </AppLayout>
    </ProtectedRoute>
  );
}

function AdminLayout() {
  return (
    <AdminRoute>
      <Outlet />
    </AdminRoute>
  );
}

export default function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <AuthProvider>
          <Routes>
            <Route path="/login" element={<LoginPage />} />

            <Route element={<AuthenticatedLayout />}>
              <Route path="/" element={<CommandListPage />} />
              <Route path="/commands/:id" element={<CommandDetailPage />} />
              <Route
                path="/commands/:id/execute"
                element={<ExecutionFormPage />}
              />
              <Route path="/executions" element={<ExecutionHistoryPage />} />
              <Route
                path="/executions/:id"
                element={<ExecutionDetailPage />}
              />
              <Route path="/notifications" element={<NotificationsPage />} />
              <Route path="/schedules" element={<ScheduleListPage />} />

              <Route element={<AdminLayout />}>
                <Route path="/admin" element={<AdminPanelPage />} />
                <Route
                  path="/admin/commands"
                  element={<CommandManagementPage />}
                />
                <Route path="/admin/users" element={<UserManagementPage />} />
              </Route>
            </Route>
          </Routes>
        </AuthProvider>
      </BrowserRouter>
    </QueryClientProvider>
  );
}
