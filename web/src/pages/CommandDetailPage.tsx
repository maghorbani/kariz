import { useParams, useNavigate } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import {
  Alert,
  Button,
  Card,
  Descriptions,
  Space,
  Spin,
  Table,
  Tag,
  Typography,
} from 'antd';
import {
  PlayCircleOutlined,
  ArrowLeftOutlined,
  ClockCircleOutlined,
} from '@ant-design/icons';
import { get } from '@/api/client';
import type { CommandEntry, ParameterDefinition } from '@/types';

const { Title } = Typography;

const ROLE_COLORS: Record<string, string> = {
  admin: 'red',
  qa: 'blue',
  data: 'green',
  operations: 'orange',
};

const parameterColumns = [
  {
    title: 'Name',
    dataIndex: 'name',
    key: 'name',
  },
  {
    title: 'Type',
    dataIndex: 'type',
    key: 'type',
    render: (type: string) => <Tag>{type}</Tag>,
  },
  {
    title: 'Required',
    dataIndex: 'required',
    key: 'required',
    render: (required: boolean) =>
      required ? <Tag color="red">Yes</Tag> : <Tag>No</Tag>,
  },
  {
    title: 'Default',
    dataIndex: 'default_value',
    key: 'default_value',
    render: (val: unknown) =>
      val !== undefined && val !== null ? String(val) : '—',
  },
  {
    title: 'Description',
    dataIndex: 'description',
    key: 'description',
  },
  {
    title: 'Validation',
    key: 'validation',
    render: (_: unknown, record: ParameterDefinition) => {
      if (!record.validation) return '—';
      const parts: string[] = [];
      if (record.validation.pattern)
        parts.push(`pattern: ${record.validation.pattern}`);
      if (record.validation.min !== undefined)
        parts.push(`min: ${record.validation.min}`);
      if (record.validation.max !== undefined)
        parts.push(`max: ${record.validation.max}`);
      if (record.validation.max_length !== undefined)
        parts.push(`maxLen: ${record.validation.max_length}`);
      if (record.validation.enum_values)
        parts.push(`enum: [${record.validation.enum_values.join(', ')}]`);
      return parts.length > 0 ? parts.join('; ') : '—';
    },
  },
  {
    title: 'Env Var Mapping',
    key: 'env_var_mapping',
    render: (_: unknown, record: ParameterDefinition) => {
      if (!record.env_var_mapping) return '—';
      return (
        <Tag color="purple">
          {record.env_var_mapping.source_container}:{record.env_var_mapping.env_var_name}
        </Tag>
      );
    },
  },
];

export default function CommandDetailPage() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();

  const {
    data: command,
    isLoading,
    error,
  } = useQuery<CommandEntry>({
    queryKey: ['command', id],
    queryFn: () => get<CommandEntry>(`/commands/${id}`),
    enabled: !!id,
  });

  if (isLoading) {
    return (
      <div
        style={{
          display: 'flex',
          justifyContent: 'center',
          alignItems: 'center',
          minHeight: '60vh',
        }}
      >
        <Spin size="large" />
      </div>
    );
  }

  if (error) {
    return (
      <div style={{ padding: 24, maxWidth: 1000, margin: '0 auto' }}>
        <Alert
          message="Failed to load command"
          description={
            error instanceof Error
              ? error.message
              : 'An unexpected error occurred.'
          }
          type="error"
          showIcon
        />
        <Button
          style={{ marginTop: 16 }}
          icon={<ArrowLeftOutlined />}
          onClick={() => navigate('/')}
        >
          Back to Commands
        </Button>
      </div>
    );
  }

  if (!command) {
    return (
      <div style={{ padding: 24, maxWidth: 1000, margin: '0 auto' }}>
        <Alert message="Command not found" type="warning" showIcon />
        <Button
          style={{ marginTop: 16 }}
          icon={<ArrowLeftOutlined />}
          onClick={() => navigate('/')}
        >
          Back to Commands
        </Button>
      </div>
    );
  }

  return (
    <div style={{ padding: 24, maxWidth: 1000, margin: '0 auto' }}>
      <Space style={{ marginBottom: 16 }}>
        <Button
          icon={<ArrowLeftOutlined />}
          onClick={() => navigate('/')}
        >
          Back
        </Button>
      </Space>

      <Card>
        <Title level={3}>{command.name}</Title>

        <Descriptions bordered column={2} style={{ marginBottom: 24 }}>
          <Descriptions.Item label="Description" span={2}>
            {command.description}
          </Descriptions.Item>
          <Descriptions.Item label="Category">
            <Tag color="blue">{command.category}</Tag>
          </Descriptions.Item>
          <Descriptions.Item label="Docker Image">
            <code>{command.docker_image}</code>
          </Descriptions.Item>
          <Descriptions.Item label="Command String" span={2}>
            <code>{command.command_string}</code>
          </Descriptions.Item>
          <Descriptions.Item label="Execution Mode">
            <Tag color={command.execution_mode === 'exec' ? 'orange' : 'green'}>
              {command.execution_mode}
            </Tag>
          </Descriptions.Item>
          <Descriptions.Item label="Timeout">
            {command.timeout_seconds}s
          </Descriptions.Item>
          <Descriptions.Item label="Concurrent Execution">
            {command.allow_concurrent ? (
              <Tag color="green">Allowed</Tag>
            ) : (
              <Tag color="default">Not Allowed</Tag>
            )}
          </Descriptions.Item>
          <Descriptions.Item label="Allowed Roles">
            <Space size={[4, 8]} wrap>
              {command.allowed_roles.map((role) => (
                <Tag key={role} color={ROLE_COLORS[role] ?? 'default'}>
                  {role}
                </Tag>
              ))}
            </Space>
          </Descriptions.Item>
        </Descriptions>

        {command.parameter_schema?.parameters?.length > 0 && (
          <>
            <Title level={5}>Parameter Schema</Title>
            <Table
              dataSource={command.parameter_schema.parameters}
              columns={parameterColumns}
              rowKey="name"
              pagination={false}
              size="small"
              style={{ marginBottom: 24 }}
            />
          </>
        )}

        <Button
          type="primary"
          size="large"
          icon={<PlayCircleOutlined />}
          onClick={() => navigate(`/commands/${command.id}/execute`)}
        >
          Execute
        </Button>
        <Button
          size="large"
          icon={<ClockCircleOutlined />}
          onClick={() => navigate(`/schedules?command_id=${command.id}`)}
          style={{ marginLeft: 8 }}
        >
          Schedule
        </Button>
      </Card>
    </div>
  );
}
