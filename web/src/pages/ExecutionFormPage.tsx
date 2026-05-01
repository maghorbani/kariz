import { useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import {
  Alert,
  Button,
  Card,
  Form,
  Input,
  InputNumber,
  Select,
  Space,
  Spin,
  Switch,
  Tag,
  Tooltip,
  Typography,
} from 'antd';
import {
  ArrowLeftOutlined,
  PlayCircleOutlined,
} from '@ant-design/icons';
import { get, post } from '@/api/client';
import type { CommandEntry, ExecutionRecord, ParameterDefinition } from '@/types';

const { Title, Text } = Typography;

function buildFormRules(param: ParameterDefinition) {
  const rules: Array<Record<string, unknown>> = [];

  // If the parameter has an env var mapping, it will be resolved server-side,
  // so don't require it on the client even if marked as required.
  if (param.required && !param.env_var_mapping) {
    rules.push({
      required: true,
      message: `${param.name} is required`,
    });
  }

  if (param.validation) {
    const v = param.validation;

    if (v.pattern) {
      rules.push({
        pattern: new RegExp(v.pattern),
        message: `${param.name} must match pattern: ${v.pattern}`,
      });
    }

    if (v.max_length !== undefined) {
      rules.push({
        max: v.max_length,
        message: `${param.name} must be at most ${v.max_length} characters`,
      });
    }
  }

  return rules;
}

function renderField(param: ParameterDefinition) {
  // Enum type with enum_values → Select
  if (param.type === 'enum' && param.validation?.enum_values) {
    return (
      <Select
        placeholder={`Select ${param.name}`}
        allowClear={!param.required}
        options={param.validation.enum_values.map((v) => ({
          label: v,
          value: v,
        }))}
      />
    );
  }

  switch (param.type) {
    case 'number':
      return (
        <InputNumber
          style={{ width: '100%' }}
          placeholder={`Enter ${param.name}`}
          min={param.validation?.min}
          max={param.validation?.max}
        />
      );
    case 'boolean':
      return <Switch />;
    case 'string':
    default:
      return <Input placeholder={`Enter ${param.name}`} />;
  }
}

export default function ExecutionFormPage() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const [form] = Form.useForm();
  const [submitting, setSubmitting] = useState(false);
  const [submitError, setSubmitError] = useState<string | null>(null);

  const {
    data: command,
    isLoading,
    error: fetchError,
  } = useQuery<CommandEntry>({
    queryKey: ['command', id],
    queryFn: () => get<CommandEntry>(`/commands/${id}`),
    enabled: !!id,
  });

  const handleSubmit = async (values: Record<string, unknown>) => {
    if (!command) return;

    setSubmitError(null);
    setSubmitting(true);

    try {
      // Clean up the parameters: remove undefined values, coerce types
      const parameters: Record<string, unknown> = {};
      for (const param of command.parameter_schema?.parameters ?? []) {
        const val = values[param.name];
        if (val !== undefined && val !== null && val !== '') {
          parameters[param.name] = val;
        }
      }

      const execution = await post<ExecutionRecord>(
        `/commands/${command.id}/execute`,
        { parameters },
      );
      navigate(`/executions/${execution.id}`, { replace: true });
    } catch (err: unknown) {
      if (err && typeof err === 'object' && 'body' in err) {
        const body = (err as { body: unknown }).body;
        if (body && typeof body === 'object' && 'message' in body) {
          setSubmitError((body as { message: string }).message);
        } else {
          setSubmitError('Failed to execute command.');
        }
      } else {
        setSubmitError('Failed to execute command.');
      }
    } finally {
      setSubmitting(false);
    }
  };

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

  if (fetchError) {
    return (
      <div style={{ padding: 24, maxWidth: 800, margin: '0 auto' }}>
        <Alert
          message="Failed to load command"
          description={
            fetchError instanceof Error
              ? fetchError.message
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
      <div style={{ padding: 24, maxWidth: 800, margin: '0 auto' }}>
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

  const parameters = command.parameter_schema?.parameters ?? [];

  // Build initial values from default_value fields
  const initialValues: Record<string, unknown> = {};
  for (const param of parameters) {
    if (param.default_value !== undefined && param.default_value !== null) {
      initialValues[param.name] = param.default_value;
    }
    // Boolean fields default to false if no default
    if (param.type === 'boolean' && initialValues[param.name] === undefined) {
      initialValues[param.name] = false;
    }
  }

  return (
    <div style={{ padding: 24, maxWidth: 800, margin: '0 auto' }}>
      <Space style={{ marginBottom: 16 }}>
        <Button
          icon={<ArrowLeftOutlined />}
          onClick={() => navigate(`/commands/${command.id}`)}
        >
          Back to Command
        </Button>
      </Space>

      <Card>
        <Title level={3}>Execute: {command.name}</Title>
        <Text type="secondary" style={{ display: 'block', marginBottom: 24 }}>
          {command.description}
        </Text>

        {submitError && (
          <Alert
            message="Execution Failed"
            description={submitError}
            type="error"
            showIcon
            closable
            onClose={() => setSubmitError(null)}
            style={{ marginBottom: 16 }}
          />
        )}

        <Form
          form={form}
          layout="vertical"
          onFinish={handleSubmit}
          initialValues={initialValues}
        >
          {parameters.length === 0 && (
            <Text type="secondary" style={{ display: 'block', marginBottom: 16 }}>
              This command has no parameters.
            </Text>
          )}

          {parameters.map((param) => (
            <Form.Item
              key={param.name}
              name={param.name}
              label={
                <span>
                  {param.name}
                  {param.description && (
                    <Text
                      type="secondary"
                      style={{ marginLeft: 8, fontWeight: 'normal' }}
                    >
                      — {param.description}
                    </Text>
                  )}
                  {param.env_var_mapping && (
                    <Tooltip title={`Auto-filled from ${param.env_var_mapping.source_container}:${param.env_var_mapping.env_var_name}. You can override this value.`}>
                      <Tag color="purple" style={{ marginLeft: 8 }}>
                        ENV: {param.env_var_mapping.env_var_name}
                      </Tag>
                    </Tooltip>
                  )}
                </span>
              }
              rules={buildFormRules(param)}
              valuePropName={param.type === 'boolean' ? 'checked' : 'value'}
            >
              {renderField(param)}
            </Form.Item>
          ))}

          <Form.Item style={{ marginTop: 24 }}>
            <Button
              type="primary"
              htmlType="submit"
              size="large"
              icon={<PlayCircleOutlined />}
              loading={submitting}
            >
              Execute Command
            </Button>
          </Form.Item>
        </Form>
      </Card>
    </div>
  );
}
