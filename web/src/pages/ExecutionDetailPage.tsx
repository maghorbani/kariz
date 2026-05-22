import { useCallback, useEffect, useRef, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  Alert,
  Button,
  Card,
  Descriptions,
  Popconfirm,
  Space,
  Spin,
  Table,
  Tag,
  Typography,
  message,
} from 'antd';
import {
  ArrowLeftOutlined,
  DownloadOutlined,
  ReloadOutlined,
  StopOutlined,
} from '@ant-design/icons';
import { ApiError, get, post } from '@/api/client';
import type { ExecutionRecord, ExecutionArtifact, ExecutionStatus } from '@/types';
import ExecutionStatusBadge from '@/components/ExecutionStatusBadge';

const { Title, Text } = Typography;

const TERMINAL_STATUSES: ExecutionStatus[] = [
  'completed',
  'failed',
  'timed_out',
  'cancelled',
];

function isTerminal(status: ExecutionStatus): boolean {
  return TERMINAL_STATUSES.includes(status);
}

function formatDuration(startedAt?: string, completedAt?: string): string {
  if (!startedAt) return '—';
  const start = new Date(startedAt).getTime();
  const end = completedAt ? new Date(completedAt).getTime() : Date.now();
  const diffMs = end - start;
  if (diffMs < 1000) return `${diffMs}ms`;
  const seconds = Math.floor(diffMs / 1000);
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  const remainSec = seconds % 60;
  if (minutes < 60) return `${minutes}m ${remainSec}s`;
  const hours = Math.floor(minutes / 60);
  const remainMin = minutes % 60;
  return `${hours}h ${remainMin}m`;
}

function formatTime(iso?: string): string {
  if (!iso) return '—';
  return new Date(iso).toLocaleString();
}

export default function ExecutionDetailPage() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const queryClient = useQueryClient();

  const [outputLines, setOutputLines] = useState<string[]>([]);
  const [sseConnected, setSseConnected] = useState(false);
  const [sseError, setSseError] = useState<string | null>(null);
  const [tick, setTick] = useState(0);
  const lastEventIdRef = useRef<string>('');
  const eventSourceRef = useRef<EventSource | null>(null);
  const terminalRef = useRef<HTMLPreElement | null>(null);

  const {
    data: execution,
    isLoading,
    error: fetchError,
    refetch,
  } = useQuery<ExecutionRecord>({
    queryKey: ['execution', id],
    queryFn: () => get<ExecutionRecord>(`/executions/${id}`),
    enabled: !!id,
    refetchInterval: (query) => {
      const data = query.state.data;
      if (data && !isTerminal(data.status)) return 5000;
      return false;
    },
  });

  const { data: artifacts } = useQuery<ExecutionArtifact[]>({
    queryKey: ['execution-artifacts', id],
    queryFn: () => get<ExecutionArtifact[]>(`/executions/${id}/artifacts`),
    enabled: !!id && !!execution && isTerminal(execution.status),
  });

  const cancelMutation = useMutation({
    mutationFn: () => post(`/executions/${id}/cancel`),
    onSuccess: () => {
      message.success('Execution cancelled');
      queryClient.invalidateQueries({ queryKey: ['execution', id] });
      refetch();
    },
    onError: (err: unknown) => {
      const msg =
        err instanceof ApiError &&
        err.body &&
        typeof err.body === 'object' &&
        'message' in err.body
          ? (err.body as { message: string }).message
          : 'Failed to cancel execution';
      message.error(msg);
    },
  });

  const scrollToBottom = useCallback(() => {
    if (terminalRef.current) {
      terminalRef.current.scrollTop = terminalRef.current.scrollHeight;
    }
  }, []);

  useEffect(() => {
    scrollToBottom();
  }, [outputLines, scrollToBottom]);

  // Live duration ticker while running
  useEffect(() => {
    if (!execution || isTerminal(execution.status)) return;
    const interval = setInterval(() => setTick((t) => t + 1), 1000);
    return () => clearInterval(interval);
  }, [execution?.status, execution]);

  const closeStream = useCallback(() => {
    if (eventSourceRef.current) {
      eventSourceRef.current.close();
      eventSourceRef.current = null;
    }
    setSseConnected(false);
  }, []);

  const connectStream = useCallback(() => {
    if (!id) return;
    closeStream();
    setSseError(null);

    const sseUrl = `/api/executions/${id}/stream`;
    const eventSource = new EventSource(sseUrl);
    eventSourceRef.current = eventSource;

    eventSource.onopen = () => {
      setSseConnected(true);
      setSseError(null);
    };

    eventSource.addEventListener('output', (event: MessageEvent) => {
      if (event.lastEventId) {
        lastEventIdRef.current = event.lastEventId;
      }
      try {
        const chunk = JSON.parse(event.data);
        const text = typeof chunk === 'string' ? chunk : chunk.data ?? '';
        if (text) {
          setOutputLines((prev) => [...prev, text]);
        }
      } catch {
        if (event.data) {
          setOutputLines((prev) => [...prev, event.data]);
        }
      }
    });

    eventSource.addEventListener('complete', () => {
      closeStream();
      refetch();
    });

    eventSource.onerror = () => {
      setSseConnected(false);
      if (eventSource.readyState === EventSource.CLOSED) {
        setSseError('Stream connection lost. Use Reconnect to resume.');
      }
    };
  }, [id, closeStream, refetch]);

  useEffect(() => {
    if (!id || !execution) return;
    if (isTerminal(execution.status)) {
      closeStream();
      const lines: string[] = [];
      if (execution.stdout) lines.push(execution.stdout);
      if (execution.stderr) lines.push(execution.stderr);
      setOutputLines(lines);
      return;
    }

    connectStream();
    return () => closeStream();
  }, [id, execution?.status, execution, connectStream, closeStream]);

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
      <div style={{ maxWidth: 1000, margin: '0 auto' }}>
        <Alert
          message="Failed to load execution"
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
          onClick={() => navigate('/executions')}
        >
          Back to History
        </Button>
      </div>
    );
  }

  if (!execution) {
    return (
      <div style={{ maxWidth: 1000, margin: '0 auto' }}>
        <Alert message="Execution not found" type="warning" showIcon />
        <Button
          style={{ marginTop: 16 }}
          icon={<ArrowLeftOutlined />}
          onClick={() => navigate('/executions')}
        >
          Back to History
        </Button>
      </div>
    );
  }

  const terminalOutput = outputLines.join('');
  const isRunning =
    execution.status === 'running' || execution.status === 'queued';
  void tick;

  return (
    <div style={{ maxWidth: 1000, margin: '0 auto' }}>
      <Space style={{ marginBottom: 16 }} wrap>
        <Button
          icon={<ArrowLeftOutlined />}
          onClick={() => navigate('/executions')}
        >
          Back to History
        </Button>
        {isRunning && (
          <Popconfirm
            title="Cancel this execution?"
            onConfirm={() => cancelMutation.mutate()}
            okText="Cancel execution"
            okButtonProps={{ danger: true }}
          >
            <Button
              danger
              icon={<StopOutlined />}
              loading={cancelMutation.isPending}
            >
              Cancel
            </Button>
          </Popconfirm>
        )}
      </Space>

      <Card style={{ marginBottom: 16 }}>
        <Space
          align="center"
          style={{
            marginBottom: 16,
            justifyContent: 'space-between',
            width: '100%',
          }}
        >
          <Title level={3} style={{ margin: 0 }}>
            Execution Detail
          </Title>
          <ExecutionStatusBadge status={execution.status} />
        </Space>

        <Descriptions bordered column={2}>
          <Descriptions.Item label="Command">
            {execution.command_name}
          </Descriptions.Item>
          <Descriptions.Item label="User">
            {execution.user_id}
          </Descriptions.Item>
          <Descriptions.Item label="Status">
            <ExecutionStatusBadge status={execution.status} />
          </Descriptions.Item>
          <Descriptions.Item label="Exit Code">
            {execution.exit_code !== undefined && execution.exit_code !== null
              ? execution.exit_code
              : '—'}
          </Descriptions.Item>
          <Descriptions.Item label="Started">
            {formatTime(execution.started_at)}
          </Descriptions.Item>
          <Descriptions.Item label="Completed">
            {formatTime(execution.completed_at)}
          </Descriptions.Item>
          <Descriptions.Item label="Duration" span={2}>
            {formatDuration(execution.started_at, execution.completed_at)}
            {isRunning && (
              <Text type="secondary"> (live)</Text>
            )}
          </Descriptions.Item>
        </Descriptions>
      </Card>

      <Card
        title={
          <Space>
            <Text strong>Output</Text>
            {sseConnected && (
              <Text type="success" style={{ fontSize: 12 }}>
                ● Live
              </Text>
            )}
          </Space>
        }
        extra={
          !isTerminal(execution.status) && sseError ? (
            <Button
              size="small"
              icon={<ReloadOutlined />}
              onClick={connectStream}
            >
              Reconnect stream
            </Button>
          ) : null
        }
      >
        {sseError && (
          <Alert
            message={sseError}
            type="warning"
            showIcon
            closable
            onClose={() => setSseError(null)}
            style={{ marginBottom: 12 }}
          />
        )}

        <pre
          ref={terminalRef}
          style={{
            background: '#1e1e1e',
            color: '#d4d4d4',
            padding: 16,
            borderRadius: 6,
            fontFamily: "'Cascadia Code', 'Fira Code', 'Consolas', monospace",
            fontSize: 13,
            lineHeight: 1.5,
            maxHeight: 500,
            overflow: 'auto',
            whiteSpace: 'pre-wrap',
            wordBreak: 'break-all',
            minHeight: 200,
            margin: 0,
          }}
        >
          {terminalOutput || (
            <Text type="secondary" style={{ color: '#666' }}>
              {isTerminal(execution.status)
                ? 'No output captured.'
                : 'Waiting for output…'}
            </Text>
          )}
        </pre>
      </Card>

      {artifacts && artifacts.length > 0 && (
        <Card title="Artifacts" style={{ marginTop: 16 }}>
          <Table<ExecutionArtifact>
            dataSource={artifacts}
            rowKey="id"
            pagination={false}
            size="small"
            columns={[
              {
                title: 'Label',
                dataIndex: 'label',
                key: 'label',
              },
              {
                title: 'Filename',
                dataIndex: 'file_name',
                key: 'file_name',
                render: (name: string) => <code>{name}</code>,
              },
              {
                title: 'Size',
                dataIndex: 'file_size_bytes',
                key: 'file_size_bytes',
                render: (bytes: number) => {
                  if (!bytes) return '—';
                  if (bytes < 1024) return `${bytes} B`;
                  if (bytes < 1024 * 1024)
                    return `${(bytes / 1024).toFixed(1)} KB`;
                  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
                },
              },
              {
                title: 'Status',
                dataIndex: 'status',
                key: 'status',
                render: (status: string) => (
                  <Tag color={status === 'stored' ? 'green' : 'red'}>
                    {status}
                  </Tag>
                ),
              },
              {
                title: 'Action',
                key: 'action',
                render: (_: unknown, record: ExecutionArtifact) =>
                  record.status === 'stored' ? (
                    <Button
                      size="small"
                      icon={<DownloadOutlined />}
                      onClick={() => {
                        window.open(
                          `/api/executions/${execution.id}/artifacts/${record.id}/download`,
                          '_blank',
                        );
                      }}
                    >
                      Download
                    </Button>
                  ) : (
                    <Text type="secondary">
                      {record.error_message || 'Failed'}
                    </Text>
                  ),
              },
            ]}
          />
        </Card>
      )}
    </div>
  );
}
