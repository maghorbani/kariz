import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useSearchParams, useNavigate } from 'react-router-dom';
import {
  Alert,
  Button,
  Descriptions,
  Drawer,
  Form,
  Input,
  InputNumber,
  message,
  Modal,
  Select,
  Space,
  Switch,
  Table,
  Typography,
} from 'antd';
import {
  PlusOutlined,
  DeleteOutlined,
  EditOutlined,
  EyeOutlined,
} from '@ant-design/icons';
import { get, post, put, del } from '@/api/client';
import type { Schedule, CommandEntry, PaginatedResult } from '@/types';

const { Title } = Typography;

function formatTime(iso?: string): string {
  if (!iso) return '—';
  return new Date(iso).toLocaleString();
}

interface CreateScheduleFormValues {
  command_id: string;
  cron_expression?: string;
  interval_seconds?: number;
  parameters_json?: string;
}

export default function ScheduleListPage() {
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const commandIdFilter = searchParams.get('command_id') || '';

  const [createOpen, setCreateOpen] = useState(false);
  const [detailSchedule, setDetailSchedule] = useState<Schedule | null>(null);
  const [editSchedule, setEditSchedule] = useState<Schedule | null>(null);
  const [createForm] = Form.useForm<CreateScheduleFormValues>();
  const [editForm] = Form.useForm<{ cron_expression?: string; interval_seconds?: number; parameters_json?: string }>();

  // Fetch schedules
  const {
    data: schedules,
    isLoading,
    error,
  } = useQuery<Schedule[]>({
    queryKey: ['schedules', commandIdFilter],
    queryFn: () => {
      const params = new URLSearchParams();
      if (commandIdFilter) params.set('command_id', commandIdFilter);
      const qs = params.toString();
      return get<Schedule[]>(`/schedules${qs ? `?${qs}` : ''}`);
    },
  });

  // Fetch commands for the selector
  const { data: commandsData } = useQuery<PaginatedResult<CommandEntry>>({
    queryKey: ['commands-for-schedule'],
    queryFn: () => get<PaginatedResult<CommandEntry>>('/commands'),
  });

  const commands = commandsData?.items ?? [];

  // Create schedule
  const createMutation = useMutation({
    mutationFn: (payload: Record<string, unknown>) =>
      post<Schedule>('/schedules', payload),
    onSuccess: () => {
      message.success('Schedule created');
      queryClient.invalidateQueries({ queryKey: ['schedules'] });
      setCreateOpen(false);
      createForm.resetFields();
    },
    onError: (err: unknown) => {
      message.error(err instanceof Error ? err.message : 'Failed to create schedule');
    },
  });

  // Toggle enable/disable
  const toggleMutation = useMutation({
    mutationFn: (schedule: Schedule) =>
      post<void>(`/schedules/${schedule.id}/${schedule.is_enabled ? 'disable' : 'enable'}`),
    onSuccess: () => {
      message.success('Schedule updated');
      queryClient.invalidateQueries({ queryKey: ['schedules'] });
    },
    onError: (err: unknown) => {
      message.error(err instanceof Error ? err.message : 'Failed to toggle schedule');
    },
  });

  // Delete schedule
  const deleteMutation = useMutation({
    mutationFn: (id: string) => del(`/schedules/${id}`),
    onSuccess: () => {
      message.success('Schedule deleted');
      queryClient.invalidateQueries({ queryKey: ['schedules'] });
      setDetailSchedule(null);
    },
    onError: (err: unknown) => {
      message.error(err instanceof Error ? err.message : 'Failed to delete schedule');
    },
  });

  // Update schedule
  const updateMutation = useMutation({
    mutationFn: ({ id, payload }: { id: string; payload: Record<string, unknown> }) =>
      put<Schedule>(`/schedules/${id}`, payload),
    onSuccess: () => {
      message.success('Schedule updated');
      queryClient.invalidateQueries({ queryKey: ['schedules'] });
      setEditSchedule(null);
      editForm.resetFields();
    },
    onError: (err: unknown) => {
      message.error(err instanceof Error ? err.message : 'Failed to update schedule');
    },
  });

  const handleCreate = async () => {
    try {
      const values = await createForm.validateFields();
      const payload: Record<string, unknown> = {
        command_id: values.command_id,
      };
      if (values.cron_expression) payload.cron_expression = values.cron_expression;
      if (values.interval_seconds) payload.interval_seconds = values.interval_seconds;
      if (values.parameters_json) {
        try {
          payload.parameters = JSON.parse(values.parameters_json);
        } catch {
          message.error('Invalid JSON for parameters');
          return;
        }
      }
      createMutation.mutate(payload);
    } catch {
      // validation errors shown by form
    }
  };

  const handleEdit = async () => {
    if (!editSchedule) return;
    try {
      const values = await editForm.validateFields();
      const payload: Record<string, unknown> = {};
      if (values.cron_expression !== undefined) payload.cron_expression = values.cron_expression;
      if (values.interval_seconds !== undefined) payload.interval_seconds = values.interval_seconds;
      if (values.parameters_json) {
        try {
          payload.parameters = JSON.parse(values.parameters_json);
        } catch {
          message.error('Invalid JSON for parameters');
          return;
        }
      }
      updateMutation.mutate({ id: editSchedule.id, payload });
    } catch {
      // validation errors shown by form
    }
  };

  const openEdit = (schedule: Schedule) => {
    setEditSchedule(schedule);
    editForm.setFieldsValue({
      cron_expression: schedule.cron_expression || '',
      interval_seconds: schedule.interval_seconds,
      parameters_json: schedule.parameters ? JSON.stringify(schedule.parameters, null, 2) : '',
    });
  };

  const columns = [
    {
      title: 'Command',
      dataIndex: 'command_name',
      key: 'command_name',
    },
    {
      title: 'Cron Expression',
      dataIndex: 'cron_expression',
      key: 'cron_expression',
      render: (cron: string) => cron ? <code>{cron}</code> : '—',
    },
    {
      title: 'Interval',
      dataIndex: 'interval_seconds',
      key: 'interval_seconds',
      render: (sec?: number) => sec ? `${sec}s` : '—',
    },
    {
      title: 'Next Run',
      dataIndex: 'next_run_at',
      key: 'next_run_at',
      render: (t: string) => formatTime(t),
    },
    {
      title: 'Last Run',
      dataIndex: 'last_run_at',
      key: 'last_run_at',
      render: (t: string) => formatTime(t),
    },
    {
      title: 'Enabled',
      key: 'is_enabled',
      render: (_: unknown, record: Schedule) => (
        <Switch
          checked={record.is_enabled}
          loading={toggleMutation.isPending}
          onChange={() => toggleMutation.mutate(record)}
        />
      ),
    },
    {
      title: 'Actions',
      key: 'actions',
      render: (_: unknown, record: Schedule) => (
        <Space size={4}>
          <Button
            size="small"
            icon={<EyeOutlined />}
            onClick={() => setDetailSchedule(record)}
          >
            View
          </Button>
          <Button
            size="small"
            icon={<EditOutlined />}
            onClick={() => openEdit(record)}
          >
            Edit
          </Button>
        </Space>
      ),
    },
  ];

  return (
    <div style={{ padding: 24, maxWidth: 1200, margin: '0 auto' }}>
      <Space
        style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 16 }}
      >
        <Title level={2} style={{ margin: 0 }}>
          Schedules
        </Title>
        <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreateOpen(true)}>
          Create Schedule
        </Button>
      </Space>

      {error && (
        <Alert
          message="Failed to load schedules"
          description={error instanceof Error ? error.message : 'An unexpected error occurred.'}
          type="error"
          showIcon
          style={{ marginBottom: 16 }}
        />
      )}

      <Table
        dataSource={schedules}
        columns={columns}
        rowKey="id"
        loading={isLoading}
        pagination={{ pageSize: 20 }}
      />

      {/* Create Schedule Drawer */}
      <Drawer
        title="Create Schedule"
        width={520}
        open={createOpen}
        onClose={() => { setCreateOpen(false); createForm.resetFields(); }}
        extra={
          <Space>
            <Button onClick={() => { setCreateOpen(false); createForm.resetFields(); }}>Cancel</Button>
            <Button type="primary" onClick={handleCreate} loading={createMutation.isPending}>
              Create
            </Button>
          </Space>
        }
      >
        <Form form={createForm} layout="vertical">
          <Form.Item
            name="command_id"
            label="Command"
            rules={[{ required: true, message: 'Select a command' }]}
          >
            <Select placeholder="Select command" showSearch optionFilterProp="label">
              {(commands ?? []).map((cmd) => (
                <Select.Option key={cmd.id} value={cmd.id} label={cmd.name}>
                  {cmd.name}
                </Select.Option>
              ))}
            </Select>
          </Form.Item>
          <Form.Item name="cron_expression" label="Cron Expression">
            <Input placeholder="e.g. 0 3 * * * (every day at 3:00 AM)" />
          </Form.Item>
          <Form.Item name="interval_seconds" label="Interval (seconds)">
            <InputNumber min={1} style={{ width: '100%' }} placeholder="e.g. 3600" />
          </Form.Item>
          <Form.Item name="parameters_json" label="Parameters (JSON)">
            <Input.TextArea rows={4} placeholder='{"key": "value"}' />
          </Form.Item>
        </Form>
      </Drawer>

      {/* Edit Schedule Modal */}
      <Modal
        title="Edit Schedule"
        open={!!editSchedule}
        onCancel={() => { setEditSchedule(null); editForm.resetFields(); }}
        onOk={handleEdit}
        confirmLoading={updateMutation.isPending}
      >
        <Form form={editForm} layout="vertical">
          <Form.Item name="cron_expression" label="Cron Expression">
            <Input placeholder="e.g. 0 3 * * *" />
          </Form.Item>
          <Form.Item name="interval_seconds" label="Interval (seconds)">
            <InputNumber min={1} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="parameters_json" label="Parameters (JSON)">
            <Input.TextArea rows={4} />
          </Form.Item>
        </Form>
      </Modal>

      {/* Schedule Detail Drawer */}
      <Drawer
        title="Schedule Detail"
        width={520}
        open={!!detailSchedule}
        onClose={() => setDetailSchedule(null)}
        extra={
          <Space>
            <Button
              danger
              icon={<DeleteOutlined />}
              onClick={() => {
                if (detailSchedule) {
                  Modal.confirm({
                    title: 'Delete Schedule',
                    content: 'Are you sure you want to delete this schedule?',
                    onOk: () => deleteMutation.mutate(detailSchedule.id),
                  });
                }
              }}
              loading={deleteMutation.isPending}
            >
              Delete
            </Button>
          </Space>
        }
      >
        {detailSchedule && (
          <>
            <Descriptions bordered column={1} size="small">
              <Descriptions.Item label="Command">
                {detailSchedule.command_name}
              </Descriptions.Item>
              <Descriptions.Item label="Cron Expression">
                {detailSchedule.cron_expression ? <code>{detailSchedule.cron_expression}</code> : '—'}
              </Descriptions.Item>
              <Descriptions.Item label="Interval">
                {detailSchedule.interval_seconds ? `${detailSchedule.interval_seconds}s` : '—'}
              </Descriptions.Item>
              <Descriptions.Item label="Enabled">
                <Switch
                  checked={detailSchedule.is_enabled}
                  loading={toggleMutation.isPending}
                  onChange={() => {
                    toggleMutation.mutate(detailSchedule);
                    setDetailSchedule({ ...detailSchedule, is_enabled: !detailSchedule.is_enabled });
                  }}
                />
              </Descriptions.Item>
              <Descriptions.Item label="Next Run">
                {formatTime(detailSchedule.next_run_at)}
              </Descriptions.Item>
              <Descriptions.Item label="Last Run">
                {formatTime(detailSchedule.last_run_at)}
              </Descriptions.Item>
              <Descriptions.Item label="Created By">
                {detailSchedule.created_by_user}
              </Descriptions.Item>
              <Descriptions.Item label="Created At">
                {formatTime(detailSchedule.created_at)}
              </Descriptions.Item>
              <Descriptions.Item label="Parameters">
                {detailSchedule.parameters ? (
                  <pre style={{ margin: 0, fontSize: 12 }}>
                    {JSON.stringify(detailSchedule.parameters, null, 2)}
                  </pre>
                ) : '—'}
              </Descriptions.Item>
            </Descriptions>

            <Space style={{ marginTop: 16 }}>
              <Button
                icon={<EditOutlined />}
                onClick={() => {
                  setDetailSchedule(null);
                  openEdit(detailSchedule);
                }}
              >
                Edit
              </Button>
              <Button
                onClick={() => navigate(`/executions?command_name=${detailSchedule.command_name}`)}
              >
                View Executions
              </Button>
            </Space>
          </>
        )}
      </Drawer>
    </div>
  );
}
