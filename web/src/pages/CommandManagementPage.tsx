import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import {
  Alert,
  Button,
  Card,
  Checkbox,
  Drawer,
  Form,
  Input,
  InputNumber,
  message,
  Select,
  Space,
  Switch,
  Table,
  Tag,
  Typography,
} from 'antd';
import {
  PlusOutlined,
  EditOutlined,
  DeleteOutlined,
  MinusCircleOutlined,
  ApiOutlined,
} from '@ant-design/icons';
import { get, post, put, del } from '@/api/client';
import type { CommandEntry, PaginatedResult } from '@/types';

const { Title } = Typography;

const ALL_ROLES = ['admin', 'qa', 'data', 'operations'];
const PARAM_TYPES = ['string', 'number', 'boolean', 'enum'];

const ROLE_COLORS: Record<string, string> = {
  admin: 'red',
  qa: 'blue',
  data: 'green',
  operations: 'orange',
};

interface ParameterFormValue {
  name: string;
  type: string;
  required: boolean;
  default_value?: string;
  description: string;
  pattern?: string;
  min?: number;
  max?: number;
  max_length?: number;
  enum_values?: string[];
  env_source_container?: string;
  env_var_name?: string;
}

interface VolumeFormValue {
  host_path: string;
  container_path: string;
  read_only: boolean;
}

interface ArtifactFormValue {
  container_path: string;
  label: string;
}

interface CommandFormValues {
  name: string;
  description: string;
  category: string;
  docker_image: string;
  command_string: string;
  allowed_roles: string[];
  timeout_seconds: number;
  allow_concurrent: boolean;
  execution_mode: 'create' | 'exec';
  target_container?: string;
  cpu_shares?: number;
  memory_mb?: number;
  cpu_count?: number;
  parameters?: ParameterFormValue[];
  volumes?: VolumeFormValue[];
  artifacts?: ArtifactFormValue[];
  artifact_dest_type?: string;
  artifact_dest_bucket?: string;
  artifact_dest_path_prefix?: string;
  artifact_dest_endpoint?: string;
}

function buildPayload(values: CommandFormValues) {
  const parameters = (values.parameters ?? []).map((p) => {
    const param: Record<string, unknown> = {
      name: p.name,
      type: p.type,
      required: p.required ?? false,
      description: p.description ?? '',
    };
    if (p.default_value !== undefined && p.default_value !== '') {
      if (p.type === 'number') {
        param.default_value = Number(p.default_value);
      } else if (p.type === 'boolean') {
        param.default_value = p.default_value === 'true';
      } else {
        param.default_value = p.default_value;
      }
    }
    const validation: Record<string, unknown> = {};
    if (p.pattern) validation.pattern = p.pattern;
    if (p.min !== undefined && p.min !== null) validation.min = p.min;
    if (p.max !== undefined && p.max !== null) validation.max = p.max;
    if (p.max_length !== undefined && p.max_length !== null)
      validation.max_length = p.max_length;
    if (p.enum_values && p.enum_values.length > 0)
      validation.enum_values = p.enum_values;
    if (Object.keys(validation).length > 0) {
      param.validation = validation;
    }
    if (p.env_source_container && p.env_var_name) {
      param.env_var_mapping = {
        source_container: p.env_source_container,
        env_var_name: p.env_var_name,
      };
    }
    return param;
  });

  const resource_limits: Record<string, number> = {};
  if (values.cpu_shares !== undefined && values.cpu_shares !== null)
    resource_limits.cpu_shares = values.cpu_shares;
  if (values.memory_mb !== undefined && values.memory_mb !== null)
    resource_limits.memory_mb = values.memory_mb;
  if (values.cpu_count !== undefined && values.cpu_count !== null)
    resource_limits.cpu_count = values.cpu_count;

  return {
    name: values.name,
    description: values.description,
    category: values.category,
    docker_image: values.docker_image,
    command_string: values.command_string,
    parameter_schema: { parameters },
    allowed_roles: values.allowed_roles,
    resource_limits,
    volumes: values.volumes ?? [],
    timeout_seconds: values.timeout_seconds ?? 300,
    allow_concurrent: values.allow_concurrent ?? false,
    execution_mode: values.execution_mode ?? 'create',
    target_container: values.target_container,
    artifacts: values.artifacts ?? [],
    artifact_destination: values.artifact_dest_type
      ? {
          type: values.artifact_dest_type,
          bucket: values.artifact_dest_bucket,
          path_prefix: values.artifact_dest_path_prefix,
          endpoint: values.artifact_dest_endpoint,
        }
      : undefined,
  };
}

function commandToFormValues(cmd: CommandEntry): CommandFormValues {
  return {
    name: cmd.name,
    description: cmd.description,
    category: cmd.category,
    docker_image: cmd.docker_image,
    command_string: cmd.command_string,
    allowed_roles: cmd.allowed_roles,
    timeout_seconds: cmd.timeout_seconds,
    allow_concurrent: cmd.allow_concurrent,
    execution_mode: cmd.execution_mode,
    target_container: cmd.target_container,
    cpu_shares: cmd.resource_limits?.cpu_shares,
    memory_mb: cmd.resource_limits?.memory_mb,
    cpu_count: cmd.resource_limits?.cpu_count,
    parameters: cmd.parameter_schema?.parameters?.map((p) => ({
      name: p.name,
      type: p.type,
      required: p.required,
      default_value:
        p.default_value !== undefined && p.default_value !== null
          ? String(p.default_value)
          : undefined,
      description: p.description,
      pattern: p.validation?.pattern,
      min: p.validation?.min,
      max: p.validation?.max,
      max_length: p.validation?.max_length,
      enum_values: p.validation?.enum_values,
      env_source_container: p.env_var_mapping?.source_container,
      env_var_name: p.env_var_mapping?.env_var_name,
    })) ?? [],
    volumes: cmd.volumes ?? [],
    artifacts: cmd.artifacts ?? [],
    artifact_dest_type: cmd.artifact_destination?.type,
    artifact_dest_bucket: cmd.artifact_destination?.bucket,
    artifact_dest_path_prefix: cmd.artifact_destination?.path_prefix,
    artifact_dest_endpoint: cmd.artifact_destination?.endpoint,
  };
}

export default function CommandManagementPage() {
  const queryClient = useQueryClient();
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [editingCommand, setEditingCommand] = useState<CommandEntry | null>(null);
  const [form] = Form.useForm<CommandFormValues>();

  const {
    data: commandsData,
    isLoading,
    error,
  } = useQuery<PaginatedResult<CommandEntry>>({
    queryKey: ['admin-commands'],
    queryFn: () => get<PaginatedResult<CommandEntry>>('/commands'),
  });

  const commands = commandsData?.items ?? [];

  const createMutation = useMutation({
    mutationFn: (payload: ReturnType<typeof buildPayload>) =>
      post<CommandEntry>('/commands', payload),
    onSuccess: () => {
      message.success('Command created successfully');
      queryClient.invalidateQueries({ queryKey: ['admin-commands'] });
      closeDrawer();
    },
    onError: (err: unknown) => {
      const msg =
        err instanceof Error ? err.message : 'Failed to create command';
      message.error(msg);
    },
  });

  const updateMutation = useMutation({
    mutationFn: ({
      id,
      payload,
    }: {
      id: string;
      payload: ReturnType<typeof buildPayload>;
    }) => put<CommandEntry>(`/commands/${id}`, payload),
    onSuccess: () => {
      message.success('Command updated successfully');
      queryClient.invalidateQueries({ queryKey: ['admin-commands'] });
      closeDrawer();
    },
    onError: (err: unknown) => {
      const msg =
        err instanceof Error ? err.message : 'Failed to update command';
      message.error(msg);
    },
  });

  const toggleMutation = useMutation({
    mutationFn: (cmd: CommandEntry) => {
      if (cmd.is_active) {
        return del(`/commands/${cmd.id}`);
      }
      return put<CommandEntry>(`/commands/${cmd.id}`, {
        ...buildPayload(commandToFormValues(cmd)),
        is_active: true,
      });
    },
    onSuccess: () => {
      message.success('Command status updated');
      queryClient.invalidateQueries({ queryKey: ['admin-commands'] });
    },
    onError: (err: unknown) => {
      const msg =
        err instanceof Error ? err.message : 'Failed to update command status';
      message.error(msg);
    },
  });

  const openCreateDrawer = () => {
    setEditingCommand(null);
    form.resetFields();
    form.setFieldsValue({
      execution_mode: 'create',
      timeout_seconds: 300,
      allow_concurrent: false,
      parameters: [],
      volumes: [],
      artifacts: [],
    });
    setDrawerOpen(true);
  };

  const openEditDrawer = (cmd: CommandEntry) => {
    setEditingCommand(cmd);
    form.setFieldsValue(commandToFormValues(cmd));
    setDrawerOpen(true);
  };

  const closeDrawer = () => {
    setDrawerOpen(false);
    setEditingCommand(null);
    form.resetFields();
  };

  const handleSubmit = async () => {
    try {
      const values = await form.validateFields();
      const payload = buildPayload(values);
      if (editingCommand) {
        updateMutation.mutate({ id: editingCommand.id, payload });
      } else {
        createMutation.mutate(payload);
      }
    } catch {
      // validation errors are shown by the form
    }
  };

  const isSaving = createMutation.isPending || updateMutation.isPending;

  const columns = [
    {
      title: 'Name',
      dataIndex: 'name',
      key: 'name',
    },
    {
      title: 'Category',
      dataIndex: 'category',
      key: 'category',
      render: (cat: string) => <Tag color="blue">{cat}</Tag>,
    },
    {
      title: 'Roles',
      dataIndex: 'allowed_roles',
      key: 'allowed_roles',
      render: (roles: string[]) => (
        <Space size={[4, 4]} wrap>
          {roles.map((r) => (
            <Tag key={r} color={ROLE_COLORS[r] ?? 'default'}>
              {r}
            </Tag>
          ))}
        </Space>
      ),
    },
    {
      title: 'Mode',
      dataIndex: 'execution_mode',
      key: 'execution_mode',
      render: (mode: string) => (
        <Tag color={mode === 'exec' ? 'orange' : 'green'}>{mode}</Tag>
      ),
    },
    {
      title: 'Active',
      key: 'is_active',
      render: (_: unknown, record: CommandEntry) => (
        <Switch
          checked={record.is_active}
          loading={toggleMutation.isPending}
          onChange={() => toggleMutation.mutate(record)}
        />
      ),
    },
    {
      title: 'Actions',
      key: 'actions',
      render: (_: unknown, record: CommandEntry) => (
        <Button
          icon={<EditOutlined />}
          size="small"
          onClick={() => openEditDrawer(record)}
        >
          Edit
        </Button>
      ),
    },
  ];

  return (
    <div style={{ padding: 24, maxWidth: 1200, margin: '0 auto' }}>
      <Space
        style={{
          display: 'flex',
          justifyContent: 'space-between',
          marginBottom: 16,
        }}
      >
        <Title level={2} style={{ margin: 0 }}>
          Command Management
        </Title>
        <Button
          type="primary"
          icon={<PlusOutlined />}
          onClick={openCreateDrawer}
        >
          Create Command
        </Button>
      </Space>

      {error && (
        <Alert
          message="Failed to load commands"
          description={
            error instanceof Error ? error.message : 'An unexpected error occurred.'
          }
          type="error"
          showIcon
          style={{ marginBottom: 16 }}
        />
      )}

      <Table
        dataSource={commands}
        columns={columns}
        rowKey="id"
        loading={isLoading}
        pagination={{ pageSize: 20 }}
      />

      <Drawer
        title={editingCommand ? 'Edit Command' : 'Create Command'}
        width={720}
        open={drawerOpen}
        onClose={closeDrawer}
        extra={
          <Space>
            <Button onClick={closeDrawer}>Cancel</Button>
            <Button
              type="primary"
              onClick={handleSubmit}
              loading={isSaving}
            >
              {editingCommand ? 'Update' : 'Create'}
            </Button>
          </Space>
        }
      >
        <CommandForm form={form} />
      </Drawer>
    </div>
  );
}


function CommandForm({
  form,
}: {
  form: ReturnType<typeof Form.useForm<CommandFormValues>>[0];
}) {
  const executionMode = Form.useWatch('execution_mode', form);

  return (
    <Form
      form={form}
      layout="vertical"
      initialValues={{
        execution_mode: 'create',
        timeout_seconds: 300,
        allow_concurrent: false,
        parameters: [],
        volumes: [],
        artifacts: [],
      }}
    >
      <Title level={5}>Basic Information</Title>

      <Form.Item
        name="name"
        label="Name"
        rules={[{ required: true, message: 'Command name is required' }]}
      >
        <Input placeholder="e.g. db-dump-production" />
      </Form.Item>

      <Form.Item
        name="description"
        label="Description"
        rules={[{ required: true, message: 'Description is required' }]}
      >
        <Input.TextArea rows={2} placeholder="What does this command do?" />
      </Form.Item>

      <Form.Item
        name="category"
        label="Category"
        rules={[{ required: true, message: 'Category is required' }]}
      >
        <Input placeholder="e.g. DB Dumps, Data Extraction" />
      </Form.Item>

      <Form.Item
        name="docker_image"
        label="Docker Image"
        rules={[{ required: true, message: 'Docker image is required' }]}
      >
        <Input placeholder="e.g. postgres:16-alpine" />
      </Form.Item>

      <Form.Item
        name="command_string"
        label="Command String"
        rules={[{ required: true, message: 'Command string is required' }]}
      >
        <Input.TextArea
          rows={2}
          placeholder="e.g. pg_dump -h $HOST -U $USER $DB"
        />
      </Form.Item>

      <Form.Item
        name="execution_mode"
        label="Execution Mode"
        rules={[{ required: true }]}
      >
        <Select>
          <Select.Option value="create">Create (new container)</Select.Option>
          <Select.Option value="exec">Exec (existing container)</Select.Option>
        </Select>
      </Form.Item>

      {executionMode === 'exec' && (
        <Form.Item
          name="target_container"
          label="Target Container"
          rules={[
            {
              required: true,
              message: 'Target container is required for exec mode',
            },
          ]}
        >
          <Input placeholder="Container name or ID" />
        </Form.Item>
      )}

      <Form.Item
        name="allowed_roles"
        label="Allowed Roles"
        rules={[
          {
            required: true,
            message: 'At least one role is required',
            type: 'array',
            min: 1,
          },
        ]}
      >
        <Checkbox.Group options={ALL_ROLES} />
      </Form.Item>

      <Title level={5}>Execution Settings</Title>

      <Space size={16}>
        <Form.Item name="timeout_seconds" label="Timeout (seconds)">
          <InputNumber min={1} max={86400} style={{ width: 160 }} />
        </Form.Item>

        <Form.Item
          name="allow_concurrent"
          label="Allow Concurrent"
          valuePropName="checked"
        >
          <Switch />
        </Form.Item>
      </Space>

      <Title level={5}>Resource Limits</Title>

      <Space size={16}>
        <Form.Item name="cpu_shares" label="CPU Shares">
          <InputNumber min={0} style={{ width: 140 }} placeholder="e.g. 1024" />
        </Form.Item>
        <Form.Item name="memory_mb" label="Memory (MB)">
          <InputNumber min={0} style={{ width: 140 }} placeholder="e.g. 512" />
        </Form.Item>
        <Form.Item name="cpu_count" label="CPU Count">
          <InputNumber min={0} style={{ width: 140 }} placeholder="e.g. 2" />
        </Form.Item>
      </Space>

      <Title level={5}>Parameters</Title>

      <Form.List name="parameters">
        {(fields, { add, remove }) => (
          <>
            {fields.map(({ key, name, ...restField }) => (
              <Card
                key={key}
                size="small"
                style={{ marginBottom: 12 }}
                extra={
                  <Button
                    type="text"
                    danger
                    icon={<MinusCircleOutlined />}
                    onClick={() => remove(name)}
                  />
                }
              >
                <Space
                  size={12}
                  wrap
                  style={{ display: 'flex', width: '100%' }}
                >
                  <Form.Item
                    {...restField}
                    name={[name, 'name']}
                    label="Name"
                    rules={[{ required: true, message: 'Required' }]}
                    style={{ marginBottom: 8 }}
                  >
                    <Input placeholder="param_name" style={{ width: 160 }} />
                  </Form.Item>

                  <Form.Item
                    {...restField}
                    name={[name, 'type']}
                    label="Type"
                    rules={[{ required: true, message: 'Required' }]}
                    style={{ marginBottom: 8 }}
                  >
                    <Select style={{ width: 120 }} placeholder="Type">
                      {PARAM_TYPES.map((t) => (
                        <Select.Option key={t} value={t}>
                          {t}
                        </Select.Option>
                      ))}
                    </Select>
                  </Form.Item>

                  <Form.Item
                    {...restField}
                    name={[name, 'required']}
                    label="Required"
                    valuePropName="checked"
                    style={{ marginBottom: 8 }}
                  >
                    <Switch />
                  </Form.Item>

                  <Form.Item
                    {...restField}
                    name={[name, 'default_value']}
                    label="Default"
                    style={{ marginBottom: 8 }}
                  >
                    <Input placeholder="Default value" style={{ width: 140 }} />
                  </Form.Item>

                  <Form.Item
                    {...restField}
                    name={[name, 'description']}
                    label="Description"
                    style={{ marginBottom: 8, flex: 1, minWidth: 200 }}
                  >
                    <Input placeholder="Description" />
                  </Form.Item>
                </Space>

                <Space size={12} wrap>
                  <Form.Item
                    {...restField}
                    name={[name, 'pattern']}
                    label="Pattern"
                    style={{ marginBottom: 0 }}
                  >
                    <Input
                      placeholder="Regex pattern"
                      style={{ width: 160 }}
                    />
                  </Form.Item>
                  <Form.Item
                    {...restField}
                    name={[name, 'min']}
                    label="Min"
                    style={{ marginBottom: 0 }}
                  >
                    <InputNumber style={{ width: 100 }} />
                  </Form.Item>
                  <Form.Item
                    {...restField}
                    name={[name, 'max']}
                    label="Max"
                    style={{ marginBottom: 0 }}
                  >
                    <InputNumber style={{ width: 100 }} />
                  </Form.Item>
                  <Form.Item
                    {...restField}
                    name={[name, 'max_length']}
                    label="Max Length"
                    style={{ marginBottom: 0 }}
                  >
                    <InputNumber style={{ width: 100 }} />
                  </Form.Item>
                  <Form.Item
                    {...restField}
                    name={[name, 'enum_values']}
                    label="Enum Values"
                    style={{ marginBottom: 0, minWidth: 200 }}
                  >
                    <Select
                      mode="tags"
                      placeholder="Add values"
                      style={{ width: '100%' }}
                    />
                  </Form.Item>
                </Space>

                <Space size={12} wrap style={{ marginTop: 8 }}>
                  <ApiOutlined style={{ color: '#8c8c8c' }} />
                  <Form.Item
                    {...restField}
                    name={[name, 'env_source_container']}
                    label="Env Var Source Container"
                    style={{ marginBottom: 0 }}
                  >
                    <Input
                      placeholder="Container name"
                      style={{ width: 180 }}
                    />
                  </Form.Item>
                  <Form.Item
                    {...restField}
                    name={[name, 'env_var_name']}
                    label="Env Var Name"
                    style={{ marginBottom: 0 }}
                  >
                    <Input
                      placeholder="ENV_VAR_NAME"
                      style={{ width: 180 }}
                    />
                  </Form.Item>
                </Space>
              </Card>
            ))}
            <Button
              type="dashed"
              onClick={() =>
                add({ type: 'string', required: false, description: '' })
              }
              block
              icon={<PlusOutlined />}
              style={{ marginBottom: 16 }}
            >
              Add Parameter
            </Button>
          </>
        )}
      </Form.List>

      <Title level={5}>Volumes</Title>

      <Form.List name="volumes">
        {(fields, { add, remove }) => (
          <>
            {fields.map(({ key, name, ...restField }) => (
              <Space
                key={key}
                size={12}
                align="start"
                style={{ display: 'flex', marginBottom: 8 }}
              >
                <Form.Item
                  {...restField}
                  name={[name, 'host_path']}
                  rules={[{ required: true, message: 'Required' }]}
                  style={{ marginBottom: 0 }}
                >
                  <Input placeholder="Host path" style={{ width: 200 }} />
                </Form.Item>
                <Form.Item
                  {...restField}
                  name={[name, 'container_path']}
                  rules={[{ required: true, message: 'Required' }]}
                  style={{ marginBottom: 0 }}
                >
                  <Input
                    placeholder="Container path"
                    style={{ width: 200 }}
                  />
                </Form.Item>
                <Form.Item
                  {...restField}
                  name={[name, 'read_only']}
                  valuePropName="checked"
                  style={{ marginBottom: 0 }}
                >
                  <Switch checkedChildren="RO" unCheckedChildren="RW" />
                </Form.Item>
                <Button
                  type="text"
                  danger
                  icon={<DeleteOutlined />}
                  onClick={() => remove(name)}
                />
              </Space>
            ))}
            <Button
              type="dashed"
              onClick={() =>
                add({ host_path: '', container_path: '', read_only: false })
              }
              block
              icon={<PlusOutlined />}
            >
              Add Volume
            </Button>
          </>
        )}
      </Form.List>

      <Title level={5}>Artifacts</Title>

      <Form.List name="artifacts">
        {(fields, { add, remove }) => (
          <>
            {fields.map(({ key, name, ...restField }) => (
              <Space
                key={key}
                size={12}
                align="start"
                style={{ display: 'flex', marginBottom: 8 }}
              >
                <Form.Item
                  {...restField}
                  name={[name, 'container_path']}
                  rules={[{ required: true, message: 'Required' }]}
                  style={{ marginBottom: 0 }}
                >
                  <Input placeholder="Container path (e.g. /output/report.csv)" style={{ width: 280 }} />
                </Form.Item>
                <Form.Item
                  {...restField}
                  name={[name, 'label']}
                  rules={[{ required: true, message: 'Required' }]}
                  style={{ marginBottom: 0 }}
                >
                  <Input placeholder="Label" style={{ width: 200 }} />
                </Form.Item>
                <Button
                  type="text"
                  danger
                  icon={<DeleteOutlined />}
                  onClick={() => remove(name)}
                />
              </Space>
            ))}
            <Button
              type="dashed"
              onClick={() => add({ container_path: '', label: '' })}
              block
              icon={<PlusOutlined />}
              style={{ marginBottom: 16 }}
            >
              Add Artifact
            </Button>
          </>
        )}
      </Form.List>

      <Title level={5}>Artifact Destination</Title>

      <Form.Item name="artifact_dest_type" label="Storage Type">
        <Select allowClear placeholder="Select storage type" style={{ width: 200 }}>
          <Select.Option value="local">Local</Select.Option>
          <Select.Option value="minio">MinIO</Select.Option>
          <Select.Option value="s3">S3</Select.Option>
        </Select>
      </Form.Item>

      <Form.Item noStyle shouldUpdate={(prev, cur) => prev.artifact_dest_type !== cur.artifact_dest_type}>
        {({ getFieldValue }) => {
          const destType = getFieldValue('artifact_dest_type');
          if (!destType || destType === 'local') return null;
          return (
            <Space size={12} wrap>
              <Form.Item name="artifact_dest_bucket" label="Bucket">
                <Input placeholder="bucket-name" style={{ width: 200 }} />
              </Form.Item>
              <Form.Item name="artifact_dest_path_prefix" label="Path Prefix">
                <Input placeholder="/artifacts" style={{ width: 200 }} />
              </Form.Item>
              <Form.Item name="artifact_dest_endpoint" label="Endpoint">
                <Input placeholder="https://minio.example.com" style={{ width: 260 }} />
              </Form.Item>
            </Space>
          );
        }}
      </Form.Item>
    </Form>
  );
}
