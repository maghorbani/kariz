import { useState, useCallback } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import {
  Alert,
  Button,
  Checkbox,
  message,
  Modal,
  Space,
  Table,
  Tag,
  Typography,
} from 'antd';
import { EditOutlined } from '@ant-design/icons';
import { get, put } from '@/api/client';
import type { UserProfile } from '@/types';

const { Title } = Typography;

const ALL_ROLES = ['admin', 'qa', 'data', 'operations'];

const ROLE_COLORS: Record<string, string> = {
  admin: 'red',
  qa: 'blue',
  data: 'green',
  operations: 'orange',
};

export default function UserManagementPage() {
  const queryClient = useQueryClient();
  const [editingUser, setEditingUser] = useState<UserProfile | null>(null);
  const [selectedRoles, setSelectedRoles] = useState<string[]>([]);
  const [modalOpen, setModalOpen] = useState(false);

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
      closeModal();
    },
    onError: (err: unknown) => {
      const msg =
        err instanceof Error ? err.message : 'Failed to update roles';
      message.error(msg);
    },
  });

  const openEditModal = useCallback((user: UserProfile) => {
    setEditingUser(user);
    setSelectedRoles([...user.roles]);
    setModalOpen(true);
  }, []);

  const closeModal = useCallback(() => {
    setModalOpen(false);
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
          {roles.length === 0 && (
            <Tag color="default">No roles</Tag>
          )}
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
        <Button
          icon={<EditOutlined />}
          size="small"
          onClick={() => openEditModal(record)}
        >
          Edit Roles
        </Button>
      ),
    },
  ];

  return (
    <div style={{ padding: 24, maxWidth: 1200, margin: '0 auto' }}>
      <Title level={2}>User Management</Title>

      {error && (
        <Alert
          message="Failed to load users"
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

      <Table
        dataSource={users}
        columns={columns}
        rowKey="id"
        loading={isLoading}
        pagination={{ pageSize: 20 }}
      />

      <Modal
        title={`Edit Roles — ${editingUser?.username ?? ''}`}
        open={modalOpen}
        onOk={handleSaveRoles}
        onCancel={closeModal}
        confirmLoading={updateRolesMutation.isPending}
        okText="Save"
      >
        <div style={{ padding: '16px 0' }}>
          <Checkbox.Group
            options={ALL_ROLES.map((role) => ({
              label: (
                <Tag color={ROLE_COLORS[role] ?? 'default'}>{role}</Tag>
              ),
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
