import { useMemo, useState, useCallback } from 'react';
import { useNavigate } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import {
  Card,
  Col,
  Empty,
  Input,
  Row,
  Spin,
  Tag,
  Typography,
  Badge,
  Space,
  Alert,
} from 'antd';
import { SearchOutlined } from '@ant-design/icons';
import { get } from '@/api/client';
import type { CommandEntry, PaginatedResult } from '@/types';

const { Title, Paragraph } = Typography;

const ROLE_COLORS: Record<string, string> = {
  admin: 'red',
  qa: 'blue',
  data: 'green',
  operations: 'orange',
};

const CATEGORY_COLORS: Record<string, string> = {
  'DB Dumps': 'purple',
  'Data Extraction': 'cyan',
  Reconciliation: 'geekblue',
};

function getCategoryColor(category: string): string {
  return CATEGORY_COLORS[category] ?? 'default';
}

export default function CommandListPage() {
  const navigate = useNavigate();
  const [searchValue, setSearchValue] = useState('');
  const [debouncedSearch, setDebouncedSearch] = useState('');
  const debounceRef = useMemo<{ timer: ReturnType<typeof setTimeout> | null }>(
    () => ({ timer: null }),
    [],
  );

  const handleSearchChange = useCallback(
    (value: string) => {
      setSearchValue(value);
      if (debounceRef.timer) {
        clearTimeout(debounceRef.timer);
      }
      debounceRef.timer = setTimeout(() => {
        setDebouncedSearch(value);
      }, 300);
    },
    [debounceRef],
  );

  const {
    data: commandsData,
    isLoading,
    error,
  } = useQuery<PaginatedResult<CommandEntry>>({
    queryKey: ['commands', debouncedSearch],
    queryFn: () => {
      const params = debouncedSearch
        ? `?search=${encodeURIComponent(debouncedSearch)}`
        : '';
      return get<PaginatedResult<CommandEntry>>(`/commands${params}`);
    },
  });

  const commands = commandsData?.items ?? [];

  const groupedByCategory = useMemo(() => {
    if (!commands || commands.length === 0) return {};
    const groups: Record<string, CommandEntry[]> = {};
    for (const cmd of commands) {
      const cat = cmd.category || 'Uncategorized';
      if (!groups[cat]) {
        groups[cat] = [];
      }
      groups[cat].push(cmd);
    }
    return groups;
  }, [commands]);

  const categories = useMemo(
    () => Object.keys(groupedByCategory).sort(),
    [groupedByCategory],
  );

  return (
    <div style={{ padding: 24, maxWidth: 1200, margin: '0 auto' }}>
      <Title level={2}>Commands</Title>

      <Input.Search
        placeholder="Search commands by name or description..."
        prefix={<SearchOutlined />}
        allowClear
        size="large"
        value={searchValue}
        onChange={(e) => handleSearchChange(e.target.value)}
        onSearch={(value) => {
          if (debounceRef.timer) {
            clearTimeout(debounceRef.timer);
          }
          setDebouncedSearch(value);
        }}
        style={{ marginBottom: 24 }}
      />

      {isLoading && (
        <div
          style={{
            display: 'flex',
            justifyContent: 'center',
            padding: 64,
          }}
        >
          <Spin size="large" />
        </div>
      )}

      {error && (
        <Alert
          message="Failed to load commands"
          description={
            error instanceof Error ? error.message : 'An unexpected error occurred.'
          }
          type="error"
          showIcon
          style={{ marginBottom: 24 }}
        />
      )}

      {!isLoading && !error && commands && commands.length === 0 && (
        <Empty
          description={
            debouncedSearch
              ? 'No commands found'
              : 'No commands available'
          }
        />
      )}

      {!isLoading &&
        !error &&
        categories.map((category) => (
          <div key={category} style={{ marginBottom: 32 }}>
            <Title level={4}>
              <Badge
                color={getCategoryColor(category)}
                text={category}
              />
            </Title>
            <Row gutter={[16, 16]}>
              {groupedByCategory[category].map((cmd) => (
                <Col key={cmd.id} xs={24} sm={12} lg={8}>
                  <Card
                    hoverable
                    onClick={() => navigate(`/commands/${cmd.id}`)}
                    style={{ height: '100%' }}
                  >
                    <Card.Meta
                      title={cmd.name}
                      description={
                        <Paragraph
                          ellipsis={{ rows: 2 }}
                          style={{ marginBottom: 12 }}
                        >
                          {cmd.description}
                        </Paragraph>
                      }
                    />
                    <Space size={[4, 8]} wrap style={{ marginTop: 8 }}>
                      {cmd.allowed_roles.map((role) => (
                        <Tag
                          key={role}
                          color={ROLE_COLORS[role] ?? 'default'}
                        >
                          {role}
                        </Tag>
                      ))}
                    </Space>
                  </Card>
                </Col>
              ))}
            </Row>
          </div>
        ))}
    </div>
  );
}
