import React, { useState, useEffect } from 'react';
import { Form, Input, Button, message, Typography } from 'antd';
import { UserOutlined, LockOutlined } from '@ant-design/icons';
import { useNavigate, Link } from 'react-router-dom';
import api from '../api';
import logo from '../assets/modelgate.png';

const { Title, Text, Paragraph } = Typography;

const Login: React.FC = () => {
  const [loading, setLoading] = useState(false);
  const [registrationEnabled, setRegistrationEnabled] = useState(false);
  const navigate = useNavigate();
  const [messageApi, contextHolder] = message.useMessage();

  useEffect(() => {
    const token = localStorage.getItem('token');
    if (token) {
      navigate('/chat');
    }
    api.get('/api/v1/config/frontend').then(res => {
      setRegistrationEnabled(res.data.data?.registration_enabled || false);
    }).catch(() => {});
  }, [navigate]);

  const onFinish = async (values: { email: string; password: string }) => {
    setLoading(true);
    try {
      const res = await api.post('/api/v1/auth/login', values);
      localStorage.setItem('token', res.data.data.token);
      localStorage.setItem('user', JSON.stringify(res.data.data.user));
      messageApi.success('登录成功');
      navigate('/chat');
    } catch (err: any) {
      const status = err.response?.status;
      const errMsg = err.response?.data?.error;
      if (status === 403 && errMsg === 'account disabled') {
        messageApi.warning('您的账号正在等待管理员审核，请稍后再试');
      } else {
        messageApi.error(errMsg || '登录失败');
      }
    } finally {
      setLoading(false);
    }
  };

  return (
    <div style={{
      height: '100vh',
      display: 'flex',
      background: '#f5f7fa',
    }}>
      {contextHolder}
      {/* 左侧品牌面板 */}
      <div style={{
        flex: '0 0 45%',
        background: 'linear-gradient(135deg, #0f0c29 0%, #302b63 50%, #24243e 100%)',
        display: 'flex',
        flexDirection: 'column',
        justifyContent: 'center',
        alignItems: 'center',
        padding: '60px',
        position: 'relative',
        overflow: 'hidden',
      }}>
        {/* 装饰性光圈 */}
        <div style={{
          position: 'absolute',
          top: '-20%',
          left: '-10%',
          width: '400px',
          height: '400px',
          background: 'radial-gradient(circle, rgba(99, 102, 241, 0.15) 0%, transparent 70%)',
          borderRadius: '50%',
        }} />
        <div style={{
          position: 'absolute',
          bottom: '-15%',
          right: '-5%',
          width: '350px',
          height: '350px',
          background: 'radial-gradient(circle, rgba(139, 92, 246, 0.12) 0%, transparent 70%)',
          borderRadius: '50%',
        }} />

        <div style={{ position: 'relative', zIndex: 1, textAlign: 'center', maxWidth: '400px' }}>
          <img
            src={logo}
            alt="Model Gate"
            style={{
              width: '80px',
              height: '80px',
              marginBottom: '32px',
              filter: 'drop-shadow(0 4px 20px rgba(139, 92, 246, 0.3))',
            }}
          />
          <Title level={2} style={{ color: '#fff', marginBottom: '16px', fontWeight: 600 }}>
            模界
          </Title>
          <Title level={4} style={{ color: 'rgba(255,255,255,0.7)', fontWeight: 400, marginTop: 0 }}>
            Model Gate
          </Title>
          <Paragraph style={{
            color: 'rgba(255,255,255,0.5)',
            fontSize: '15px',
            lineHeight: '1.8',
            marginTop: '32px',
          }}>
            企业级大模型统一接入网关
            <br />
            多后端负载均衡 · 配额管控 · 审计追踪
          </Paragraph>

          {/* 特性标签 */}
          <div style={{
            display: 'flex',
            gap: '12px',
            flexWrap: 'wrap',
            justifyContent: 'center',
            marginTop: '40px',
          }}>
            {['OpenAI 兼容', 'Anthropic 兼容', 'SSO 支持', '多后端'].map(tag => (
              <span key={tag} style={{
                padding: '6px 16px',
                borderRadius: '20px',
                background: 'rgba(255,255,255,0.08)',
                border: '1px solid rgba(255,255,255,0.12)',
                color: 'rgba(255,255,255,0.6)',
                fontSize: '13px',
                backdropFilter: 'blur(10px)',
              }}>
                {tag}
              </span>
            ))}
          </div>
        </div>
      </div>

      {/* 右侧登录面板 */}
      <div style={{
        flex: 1,
        display: 'flex',
        flexDirection: 'column',
        justifyContent: 'center',
        alignItems: 'center',
        padding: '60px',
      }}>
        <div style={{ width: '100%', maxWidth: '380px' }}>
          <div style={{ marginBottom: '40px' }}>
            <Title level={3} style={{ marginBottom: '8px', color: '#1a1a2e' }}>
              登录账号
            </Title>
            <Text type="secondary">请输入您的账号信息以继续</Text>
          </div>

          <Form
            onFinish={onFinish}
            size="large"
            layout="vertical"
          >
            <Form.Item
              name="email"
              label="邮箱"
              rules={[{ required: true, message: '请输入邮箱' }]}
            >
              <Input
                prefix={<UserOutlined style={{ color: '#bfbfbf' }} />}
                placeholder="请输入邮箱地址"
                style={{ borderRadius: '8px', height: '44px' }}
              />
            </Form.Item>
            <Form.Item
              name="password"
              label="密码"
              rules={[{ required: true, message: '请输入密码' }]}
            >
              <Input.Password
                prefix={<LockOutlined style={{ color: '#bfbfbf' }} />}
                placeholder="请输入密码"
                style={{ borderRadius: '8px', height: '44px' }}
              />
            </Form.Item>
            <Form.Item style={{ marginTop: '32px' }}>
              <Button
                type="primary"
                htmlType="submit"
                loading={loading}
                block
                style={{
                  height: '44px',
                  borderRadius: '8px',
                  fontWeight: 500,
                  fontSize: '15px',
                  background: 'linear-gradient(135deg, #667eea 0%, #764ba2 100%)',
                  border: 'none',
                  boxShadow: '0 4px 15px rgba(102, 126, 234, 0.35)',
                }}
              >
                登 录
              </Button>
            </Form.Item>
          </Form>

          {registrationEnabled && (
            <div style={{
              textAlign: 'center',
              marginTop: '24px',
              padding: '16px',
              borderRadius: '8px',
              background: '#f8f9ff',
            }}>
              <Text type="secondary">还没有账号？</Text>
              <Link to="/register" style={{ marginLeft: '4px', fontWeight: 500 }}>
                立即注册
              </Link>
            </div>
          )}
        </div>

        {/* 底部版权 */}
        <div style={{
          position: 'absolute',
          bottom: '24px',
          color: '#bfbfbf',
          fontSize: '12px',
        }}>
          © {new Date().getFullYear()} Model Gate · 企业大模型统一接入网关
        </div>
      </div>
    </div>
  );
};

export default Login;