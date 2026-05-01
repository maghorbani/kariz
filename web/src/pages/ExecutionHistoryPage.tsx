import { useState, useCallback, useMemo } from 'react';
import { useNavigate } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import {
  Alert,
  Button,
  DatePicker,
  Input,
  Select,
  Space,
  Table,
  Typography,
} from 'antd';
import type { TablePaginationConfig } from 'antd';
import { SearchOutlined, ClearOutlined } from '@ant-design/icons';
import { get } from '@/api/client';
import type { ExecutionRecord, ExecutionStatus, PaginatedResult } from '@/types';
import ExecutionStatusBadge from '@/components/ExecutionStatusBadge';

const { Title } = Typography;
const { RangePicker } = DatePicker;

const STATUS_OPTIONS: { label: string; value: ExecutionStatus }[] = [
  { label: 'Queued', value: 'queued' },
  { label: 'Running', value: 'running' },
  { label: 'Completed', value: 'completed' },
  { label: 'Failed', value: 'failed' },
  { label: 'Timed Out', value: 'timed_out' },
  { label: 'Cancelled', value: 'cancelled' },
];

interface Filters {
  commandName: string;
  user: string;
  status: ExecutionStatus | undefined;
  dateRange: [string, string] | null;
}

const DEFAULT_PAGE_SIZE = 20;

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
  return `${minutes}m ${remainSec}s`;
}

function formatTime(iso?: string): string {
  if (!iso) return '—';
  return new Date(iso).toLocaleString();
}

export default function ExecutionHistoryPage() {
  const navigate = useNavigate();
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(DEFAULT_PAGE_SIZE);
  const [filters, setFilters] = useState<Filters>({
    commandName: '',
    user: '',
    status: undefined,
    dateRange: null,
  });

  const buildQueryString = useCallback(() => {
    const params = new URLSearchParams();
    params.set('page', String(page));
    params.set('page_size', String(pageSize));
    if (filters.commandName) params.set('command_name', filters.commandName);
    if (filters.user) params.set('user', filters.user);
    if (filters.status) params.set('status', filters.status);
    if (filters.dateRange) {
      params.set('start_date', filters.dateRange[0]);
      params.set('end_date', filters.dateRange[1]);
    }
    return params.toString();
  }, [page, pageSize, filters]);

  const {
    data,
    isLoading,
    error,
  } = useQuery<PaginatedResult<ExecutionRecord>>({
    queryKey: ['executions', page, pageSize, filters],
    queryFn: () =>
      get<PaginatedResult<ExecutionRecord>>(`/executions?${buildQueryString()}`),
  });

  const handleTableChange = useCallback((pagination: TablePaginationConfig) => {
    setPage(pagination.current ?? 1);
    setPageSize(pagination.pageSize ?? DEFAULT_PAGE_SIZE);
  }, []);

  const handleFilterChange = useCallback(
    (key: keyof Filters, value: unknown) => {
      setFilters((prev) => ({ ...prev, [key]: value }));
      setPage(1); // Reset to first page on filter change
    },
    [],
  );

  const handleDateRangeChange = useCallback(
    (_: unknown, dateStrings: [string, string]) => {
      const hasValues = dateStrings[0] && dateStrings[1];
      handleFilterChange('dateRange', hasValues ? dateStrings : null);
    },
    [handleFilterChange],
  );

  const clearFilters = useCallback(() => {
    setFilters({
      commandName: '',
      user: '',
      status: undefined,
      dateRange: null,
    });
    setPage(1);
  }, []);

  const hasActiveFilters = useMemo(
    () =>
      filters.commandName !== '' ||
      filters.user !== '' ||
      filters.status !== undefined ||
      filters.dateRange !== null,
    [filters],
  );

  const columns = [
    {
      title: 'Command',
      dataIndex: 'command_name',
      key: 'command_name',
      ellipsis: true,
    },
    {
      title: 'User',
      dataIndex: 'user_id',
      key: 'user_id',
      ellipsis: true,
    },
    {
      title: 'Status',
      dataIndex: 'status',
      key: 'status',
      render: (status: ExecutionStatus) => (
        <ExecutionStatusBadge status={status} />
      ),
    },
    {
      title: 'Start Time',
      dataIndex: 'started_at',
      key: 'started_at',
      render: (val: string | undefined) => formatTime(val),
    },
    {
      title: 'Duration',
      key: 'duration',
      render: (_: unknown, record: ExecutionRecord) =>
        formatDuration(record.started_at, record.completed_at),
    },
  ];

  return (
    <div style={{ padding: 24, maxWidth: 1200, margin: '0 auto' }}>
      <Title level={2}>Execution History</Title>

      {/* Filter controls */}
      <Space wrap size={12} style={{ marginBottom: 16, width: '100%' }}>
        <Input
          placeholder="Filter by command name"
          prefix={<SearchOutlined />}
          allowClear
          value={filters.commandName}
          onChange={(e) => handleFilterChange('commandName', e.target.value)}
          style={{ width: 200 }}
        />
        <Input
          placeholder="Filter by user"
          allowClear
          value={filters.user}
          onChange={(e) => handleFilterChange('user', e.target.value)}
          style={{ width: 180 }}
        />
        <Select
          placeholder="Filter by status"
          allowClear
          value={filters.status}
          onChange={(val) => handleFilterChange('status', val)}
          options={STATUS_OPTIONS}
          style={{ width: 160 }}
        />
        <RangePicker
          onChange={handleDateRangeChange}
          value={null}
          style={{ width: 280 }}
        />
        {hasActiveFilters && (
          <Button icon={<ClearOutlined />} onClick={clearFilters}>
            Clear Filters
          </Button>
        )}
      </Space>

      {error && (
        <Alert
          message="Failed to load executions"
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
        dataSource={data?.items}
        columns={columns}
        rowKey="id"
        loading={isLoading}
        pagination={{
          current: page,
          pageSize,
          total: data?.total ?? 0,
          showSizeChanger: true,
          showTotal: (total) => `Total ${total} executions`,
        }}
        onChange={handleTableChange}
        onRow={(record) => ({
          onClick: () => navigate(`/executions/${record.id}`),
          style: { cursor: 'pointer' },
        })}
      />
    </div>
  );
}
