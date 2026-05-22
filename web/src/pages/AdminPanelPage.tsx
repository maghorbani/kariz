import { Link } from 'react-router-dom';
import { Card, Col, Row, Typography } from 'antd';
import {
  SettingOutlined,
  TeamOutlined,
} from '@ant-design/icons';

const { Title, Paragraph } = Typography;

export default function AdminPanelPage() {
  return (
    <div style={{ maxWidth: 900 }}>
      <Title level={2}>Admin Panel</Title>
      <Paragraph type="secondary">
        Manage commands and users for the KARIZ platform.
      </Paragraph>

      <Row gutter={[16, 16]} style={{ marginTop: 24 }}>
        <Col xs={24} sm={12}>
          <Link to="/admin/commands" style={{ display: 'block' }}>
            <Card hoverable>
              <SettingOutlined style={{ fontSize: 32, color: '#1677ff' }} />
              <Title level={4} style={{ marginTop: 16, marginBottom: 8 }}>
                Manage Commands
              </Title>
              <Paragraph type="secondary" style={{ marginBottom: 0 }}>
                Register, edit, and deactivate commands in the catalog.
              </Paragraph>
            </Card>
          </Link>
        </Col>
        <Col xs={24} sm={12}>
          <Link to="/admin/users" style={{ display: 'block' }}>
            <Card hoverable>
              <TeamOutlined style={{ fontSize: 32, color: '#1677ff' }} />
              <Title level={4} style={{ marginTop: 16, marginBottom: 8 }}>
                Manage Users
              </Title>
              <Paragraph type="secondary" style={{ marginBottom: 0 }}>
                Create users, assign roles, and deactivate accounts.
              </Paragraph>
            </Card>
          </Link>
        </Col>
      </Row>
    </div>
  );
}
