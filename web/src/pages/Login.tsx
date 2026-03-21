import React, { useState, useEffect } from 'react';
import { Form, Input, Button, Card, message } from 'antd';
import { UserOutlined, LockOutlined } from '@ant-design/icons';
import { useNavigate, Link } from 'react-router-dom';
import api from '../api';

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
    // 获取前端配置，检查是否开放注册
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
      justifyContent: 'center', 
      alignItems: 'center',
      background: '#f0f2f5'
    }}>
      <Card title="模界（Model Gate）登录" style={{ width: 400 }}>
        {contextHolder}
        <Form onFinish={onFinish}>
          <Form.Item 
            name="email" 
            rules={[{ required: true, message: '请输入邮箱' }]}
          >
            <Input prefix={<UserOutlined />} placeholder="邮箱" />
          </Form.Item>
          <Form.Item 
            name="password" 
            rules={[{ required: true, message: '请输入密码' }]}
          >
            <Input.Password prefix={<LockOutlined />} placeholder="密码" />
          </Form.Item>
          <Form.Item>
            <Button type="primary" htmlType="submit" loading={loading} block>
              登录
            </Button>
          </Form.Item>
          {registrationEnabled && (
            <div style={{ textAlign: 'center' }}>
              还没有账号？<Link to="/register">立即注册</Link>
            </div>
          )}
        </Form>
      </Card>
    </div>
  );
};

export default Login;