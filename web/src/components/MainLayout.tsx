import React, { useState, useEffect } from 'react';
import { Layout, Menu, Avatar, Dropdown } from 'antd';
import {
  MessageOutlined,
  DashboardOutlined,
  KeyOutlined,
  QuestionCircleOutlined,
  BookOutlined,
  LogoutOutlined,
  UserOutlined,
  DownOutlined,
  SettingOutlined,
} from '@ant-design/icons';
import { useNavigate, useLocation, Outlet } from 'react-router-dom';
import api from '../api';
import logo from '../assets/modelgate.png';

const { Header, Content } = Layout;

interface FrontendConfig {
  feedback_url: string;
  dev_manual_url: string;
}

const MainLayout: React.FC = () => {
  const navigate = useNavigate();
  const location = useLocation();
  const [config, setConfig] = useState<FrontendConfig | null>(null);

  const storedUser = localStorage.getItem('user');
  const user = storedUser && storedUser !== 'undefined' ? JSON.parse(storedUser) : {};

  useEffect(() => {
    const fetchConfig = async () => {
      try {
        const res = await api.get('/api/v1/config/frontend');
        setConfig(res.data.data);
      } catch (err) {
        console.error('Failed to load frontend config:', err);
      }
    };
    fetchConfig();
  }, []);

  const handleLogout = () => {
    localStorage.clear();
    navigate('/login');
  };

  const userMenuItems = [
    {
      key: 'profile',
      icon: <UserOutlined />,
      label: user.name || '用户',
      disabled: true,
    },
    {
      type: 'divider' as const,
    },
    {
      key: 'logout',
      icon: <LogoutOutlined />,
      label: '退出登录',
      onClick: handleLogout,
    },
  ];

  const getSelectedKey = () => {
    const path = location.pathname;
    if (path === '/chat' || path === '/') return 'chat';
    if (path === '/stats') return 'stats';
    if (path === '/keys') return 'keys';
    if (path === '/admin' || path.startsWith('/admin/')) return 'admin';
    return 'chat';
  };

  const menuItems = [
    {
      key: 'chat',
      icon: <MessageOutlined />,
      label: 'AI 操练场',
      onClick: () => navigate('/chat'),
    },
    {
      key: 'stats',
      icon: <DashboardOutlined />,
      label: '使用统计',
      onClick: () => navigate('/stats'),
    },
    {
      key: 'keys',
      icon: <KeyOutlined />,
      label: 'API Key 管理',
      onClick: () => navigate('/keys'),
    },
    ...(user.role === 'admin' ? [{
      key: 'admin',
      icon: <SettingOutlined />,
      label: '配置管理',
      onClick: () => navigate('/admin/users'),
    }] : []),
  ];

  const bottomMenuItems = [
    ...(config?.feedback_url ? [{
      key: 'feedback',
      icon: <QuestionCircleOutlined />,
      label: (
        <a href={config.feedback_url} target="_blank" rel="noopener noreferrer" style={{ color: 'inherit' }}>
          用户反馈
        </a>
      ),
    }] : []),
    ...(config?.dev_manual_url ? [{
      key: 'dev_manual',
      icon: <BookOutlined />,
      label: (
        <a href={config.dev_manual_url} target="_blank" rel="noopener noreferrer" style={{ color: 'inherit' }}>
          开发手册
        </a>
      ),
    }] : []),
  ];

  return (
    <div style={{ display: 'flex', height: '100vh', overflow: 'hidden' }}>
      {/* 左侧边栏 - 固定宽度，固定在屏幕左侧 */}
      <div
        style={{
          width: 200,
          minWidth: 200,
          maxWidth: 200,
          background: '#fff',
          boxShadow: '2px 0 8px rgba(0,0,0,0.06)',
          zIndex: 100,
          display: 'flex',
          flexDirection: 'column',
          height: '100vh',
        }}
      >
        <div style={{
          height: 64,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          borderBottom: '1px solid #f0f0f0',
        }}>
          <img src={logo} alt="Model Gate" style={{ height: 55 }} />
        </div>

        <div style={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'auto' }}>
          <Menu
            mode="inline"
            selectedKeys={[getSelectedKey()]}
            items={menuItems}
            style={{ flex: 1, borderRight: 0 }}
          />

          {bottomMenuItems.length > 0 && (
            <Menu
              mode="inline"
              selectable={false}
              items={bottomMenuItems}
              style={{ borderTop: '1px solid #f0f0f0', borderRight: 0 }}
            />
          )}
        </div>
      </div>

      {/* 右侧内容区域 - 占据剩余空间，只有这里有滚动条 */}
      <div style={{ flex: 1, display: 'flex', flexDirection: 'column', minWidth: 0, height: '100vh', overflow: 'hidden' }}>
        {/* Header */}
        <Header
          style={{
            background: '#fff',
            padding: '0 24px',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            boxShadow: '0 2px 8px rgba(0,0,0,0.06)',
            height: 64,
            flexShrink: 0,
          }}
        >
          <div style={{
            display: 'flex',
            alignItems: 'center',
            gap: '12px',
          }}>
            <div style={{
              fontSize: '20px',
              fontWeight: 700,
              background: 'linear-gradient(135deg, #1890ff 0%, #52c41a 100%)',
              WebkitBackgroundClip: 'text',
              WebkitTextFillColor: 'transparent',
              backgroundClip: 'text',
              letterSpacing: '0.5px',
            }}>
              模界 Model Gate
            </div>
            <div style={{
              width: '1px',
              height: '20px',
              background: '#d9d9d9',
              margin: '0 4px',
            }} />
            <div style={{
              fontSize: '14px',
              fontWeight: 400,
              color: '#666',
              letterSpacing: '0.3px',
            }}>
              <span style={{ color: '#1890ff', fontWeight: 500 }}>让 AI 触手可及</span>
              <span style={{ margin: '0 8px', color: '#d9d9d9' }}>·</span>
              <span style={{ color: '#52c41a', fontWeight: 500 }}>使能效率倍增新时代</span>
            </div>
          </div>

          <div style={{ display: 'flex', alignItems: 'center', gap: 16 }}>
            <Dropdown menu={{ items: userMenuItems }} placement="bottomRight">
              <div style={{ cursor: 'pointer', display: 'flex', alignItems: 'center', gap: 8 }}>
                <Avatar icon={<UserOutlined />} size="small" />
                <span>{user.name || '用户'}</span>
                <DownOutlined style={{ fontSize: 12 }} />
              </div>
            </Dropdown>
          </div>
        </Header>

        {/* Content - 只有这个区域可以滚动 */}
        <Content
          style={{
            flex: 1,
            padding: 24,
            background: '#f0f2f5',
            overflow: 'auto',
          }}
        >
          <Outlet />
        </Content>
      </div>
    </div>
  );
};

export default MainLayout;
