import { useState, useCallback } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import {
  Alert,
  Button,
  Checkbox,
  Form,
  Input,
  message,
  Modal,
  Popconfirm,
  Space,
  Table,
  Tag,
  Typography,
} from 'antd';
import { EditOutlined, PlusOutlined, StopOutlined } from '@ant-design/icons';
import { ApiError, get, post, put } from '@/api/client';
import type { UserProfile } from '@/types';
import { ALL_ROLES, ROLE_COLORS } from '@/utils/auth';
import { useAuth } from '@/context/AuthContext';

const { Title } = Typography;

interface CreateUserForm {
  username: string;
  password: string;
  email: string;
  roles: string[];
}

function apiErrorMessage(err: unknown, fallback: string): string {
  if (err instanceof ApiError && err.body && typeof err.body === 'object' && 'message' in err.body) {
    return (err.body as { message: string }).message;
  }
  if (err instanceof Error) return err.message;
  return fallback;
}

export default function UserManagementPage() {
  const queryClient = useQueryClient();
  const { user: currentUser } = useAuth();
  const [createForm] = Form.useForm<CreateUserForm>();
  const [editingUser, setEditingUser] = useState<UserProfile | null>(null);
  const [selectedRoles, setSelectedRoles] = useState<string[]>([]);
  const [rolesModalOpen, setRolesModalOpen] = useState(false);
  const [createModalOpen, setCreateModalOpen] = useState(false);

  const {
    data: users,
    isLoading,
    error,
  } = useQuery<UserProfile[]>({
    queryKey: ['admin-users'],
    queryFn: () => get<UserProfile[]>('/admin/users'),
  });

  const updateRolesMutation = useMutation({
    mutationFn: ({ userId, roles }: { userId: string; roles: string[] }) =>
      put(`/admin/users/${userId}/roles`, { roles }),
    onSuccess: () => {
      message.success('Roles updated successfully');
      queryClient.invalidateQueries({ queryKey: ['admin-users'] });
      closeRolesModal();
    },
    onError: (err: unknown) => {
      message.error(apiErrorMessage(err, 'Failed to update roles'));
    },
  });

  const createUserMutation = useMutation({
    mutationFn: (values: CreateUserForm) =>
      post<UserProfile>('/admin/users', values),
    onSuccess: () => {
      message.success('User created successfully');
      queryClient.invalidateQueries({ queryKey: ['admin-users'] });
      setCreateModalOpen(false);
      createForm.resetFields();
    },
    onError: (err: unknown) => {
      message.error(apiErrorMessage(err, 'Failed to create user'));
    },
  });

  const deactivateMutation = useMutation({
    mutationFn: (userId: string) =>
      post<UserProfile>(`/admin/users/${userId}/deactivate`),
    onSuccess: () => {
      message.success('User deactivated');
      queryClient.invalidateQueries({ queryKey: ['admin-users'] });
    },
    onError: (err: unknown) => {
      message.error(apiErrorMessage(err, 'Failed to deactivate user'));
    },
  });

  const openRolesModal = useCallback((user: UserProfile) => {
    setEditingUser(user);
    setSelectedRoles([...user.roles]);
    setRolesModalOpen(true);
  }, []);

  const closeRolesModal = useCallback(() => {
    setRolesModalOpen(false);
    setEditingUser(null);
    setSelectedRoles([]);
  }, []);

  const handleSaveRoles = useCallback(() => {
    if (!editingUser) return;
    updateRolesMutation.mutate({
      userId: editingUser.id,
      roles: selectedRoles,
    });
  }, [editingUser, selectedRoles, updateRolesMutation]);

  const columns = [
    {
      title: 'Username',
      dataIndex: 'username',
      key: 'username',
    },
    {
      title: 'Email',
      dataIndex: 'email',
      key: 'email',
      ellipsis: true,
    },
    {
      title: 'Roles',
      dataIndex: 'roles',
      key: 'roles',
      render: (roles: string[]) => (
        <Space size={[4, 4]} wrap>
          {roles.map((role) => (
            <Tag key={role} color={ROLE_COLORS[role] ?? 'default'}>
              {role}
            </Tag>
          ))}
          {roles.length === 0 && <Tag color="default">No roles</Tag>}
        </Space>
      ),
    },
    {
      title: 'Active',
      dataIndex: 'is_active',
      key: 'is_active',
      render: (active: boolean) => (
        <Tag color={active ? 'success' : 'default'}>
          {active ? 'Active' : 'Inactive'}
        </Tag>
      ),
    },
    {
      title: 'Actions',
      key: 'actions',
      render: (_: unknown, record: UserProfile) => (
        <Space>
          <Button
            icon={<EditOutlined />}
            size="small"
            onClick={() => openRolesModal(record)}
          >
            Edit Roles
          </Button>
          {record.is_active && record.id !== currentUser?.id && (
            <Popconfirm
              title="Deactivate this user?"
              description="They will be logged out and cannot sign in again."
              onConfirm={() => deactivateMutation.mutate(record.id)}
              okText="Deactivate"
              okButtonProps={{ danger: true }}
            >
              <Button
                icon={<StopOutlined />}
                size="small"
                danger
                loading={deactivateMutation.isPending}
              >
                Deactivate
              </Button>
            </Popconfirm>
          )}
        </Space>
      ),
    },
  ];

  return (
    <div style={{ maxWidth: 1200, margin: '0 auto' }}>
      <div
        style={{
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          marginBottom: 16,
        }}
      >
        <Title level={2} style={{ margin: 0 }}>
          User Management
        </Title>
        <Button
          type="primary"
          icon={<PlusOutlined />}
          onClick={() => setCreateModalOpen(true)}
        >
          Add User
        </Button>
      </div>

      {error && (
        <Alert
          message="Failed to load users"
          description={apiErrorMessage(error, 'An unexpected error occurred.')}
          type="error"
          showIcon
          style={{ marginBottom: 16 }}
        />
      )}

      <Table
        dataSource={users}
        columns={columns}
        rowKey="id"
        loading={isLoading}
        pagination={{ pageSize: 20 }}
      />

      <Modal
        title="Create User"
        open={createModalOpen}
        onOk={() => createForm.submit()}
        onCancel={() => {
          setCreateModalOpen(false);
          createForm.resetFields();
        }}
        confirmLoading={createUserMutation.isPending}
        okText="Create"
      >
        <Form
          form={createForm}
          layout="vertical"
          onFinish={(values) => createUserMutation.mutate(values)}
          initialValues={{ roles: ['qa'] }}
        >
          <Form.Item
            name="username"
            label="Username"
            rules={[{ required: true, message: 'Username is required' }]}
          >
            <Input />
          </Form.Item>
          <Form.Item
            name="email"
            label="Email"
          >
            <Input type="email" />
          </Form.Item>
          <Form.Item
            name="password"
            label="Password"
            rules={[{ required: true, min: 6, message: 'At least 6 characters' }]}
          >
            <Input.Password />
          </Form.Item>
          <Form.Item
            name="roles"
            label="Roles"
            rules={[{ required: true, message: 'Select at least one role' }]}
          >
            <Checkbox.Group
              options={ALL_ROLES.map((role) => ({
                label: <Tag color={ROLE_COLORS[role]}>{role}</Tag>,
                value: role,
              }))}
              style={{ display: 'flex', flexDirection: 'column', gap: 8 }}
            />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title={`Edit Roles — ${editingUser?.username ?? ''}`}
        open={rolesModalOpen}
        onOk={handleSaveRoles}
        onCancel={closeRolesModal}
        confirmLoading={updateRolesMutation.isPending}
        okText="Save"
      >
        <div style={{ padding: '16px 0' }}>
          <Checkbox.Group
            options={ALL_ROLES.map((role) => ({
              label: <Tag color={ROLE_COLORS[role] ?? 'default'}>{role}</Tag>,
              value: role,
            }))}
            value={selectedRoles}
            onChange={(checked) => setSelectedRoles(checked as string[])}
            style={{ display: 'flex', flexDirection: 'column', gap: 12 }}
          />
        </div>
      </Modal>
    </div>
  );
}
