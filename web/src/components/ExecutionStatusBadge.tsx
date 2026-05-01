import { Tag } from 'antd';
import {
  CheckCircleOutlined,
  ClockCircleOutlined,
  CloseCircleOutlined,
  ExclamationCircleOutlined,
  LoadingOutlined,
  MinusCircleOutlined,
} from '@ant-design/icons';
import type { ExecutionStatus } from '@/types';

const STATUS_CONFIG: Record<
  ExecutionStatus,
  { color: string; icon: React.ReactNode; label: string }
> = {
  queued: {
    color: 'default',
    icon: <ClockCircleOutlined />,
    label: 'Queued',
  },
  running: {
    color: 'processing',
    icon: <LoadingOutlined />,
    label: 'Running',
  },
  completed: {
    color: 'success',
    icon: <CheckCircleOutlined />,
    label: 'Completed',
  },
  failed: {
    color: 'error',
    icon: <CloseCircleOutlined />,
    label: 'Failed',
  },
  timed_out: {
    color: 'warning',
    icon: <ExclamationCircleOutlined />,
    label: 'Timed Out',
  },
  cancelled: {
    color: 'default',
    icon: <MinusCircleOutlined />,
    label: 'Cancelled',
  },
};

interface ExecutionStatusBadgeProps {
  status: ExecutionStatus;
}

export default function ExecutionStatusBadge({ status }: ExecutionStatusBadgeProps) {
  const config = STATUS_CONFIG[status] ?? STATUS_CONFIG.queued;

  return (
    <Tag color={config.color} icon={config.icon}>
      {config.label}
    </Tag>
  );
}
