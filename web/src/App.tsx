import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { BrowserRouter, Route, Routes } from 'react-router-dom';

import { AuthProvider } from './context/AuthContext';
import ProtectedRoute from './components/ProtectedRoute';

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
      staleTime: 30_000, // 30 seconds
    },
  },
});

export default function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <AuthProvider>
          <Routes>
            {/* Public */}
            <Route path="/login" element={<LoginPage />} />

            {/* Authenticated routes */}
            <Route
              path="/"
              element={
                <ProtectedRoute>
                  <CommandListPage />
                </ProtectedRoute>
              }
            />
            <Route
              path="/commands/:id"
              element={
                <ProtectedRoute>
                  <CommandDetailPage />
                </ProtectedRoute>
              }
            />
            <Route
              path="/commands/:id/execute"
              element={
                <ProtectedRoute>
                  <ExecutionFormPage />
                </ProtectedRoute>
              }
            />

            <Route
              path="/executions"
              element={
                <ProtectedRoute>
                  <ExecutionHistoryPage />
                </ProtectedRoute>
              }
            />
            <Route
              path="/executions/:id"
              element={
                <ProtectedRoute>
                  <ExecutionDetailPage />
                </ProtectedRoute>
              }
            />

            <Route
              path="/notifications"
              element={
                <ProtectedRoute>
                  <NotificationsPage />
                </ProtectedRoute>
              }
            />

            <Route
              path="/schedules"
              element={
                <ProtectedRoute>
                  <ScheduleListPage />
                </ProtectedRoute>
              }
            />

            {/* Admin routes */}
            <Route
              path="/admin"
              element={
                <ProtectedRoute>
                  <AdminPanelPage />
                </ProtectedRoute>
              }
            />
            <Route
              path="/admin/commands"
              element={
                <ProtectedRoute>
                  <CommandManagementPage />
                </ProtectedRoute>
              }
            />
            <Route
              path="/admin/users"
              element={
                <ProtectedRoute>
                  <UserManagementPage />
                </ProtectedRoute>
              }
            />
          </Routes>
        </AuthProvider>
      </BrowserRouter>
    </QueryClientProvider>
  );
}
