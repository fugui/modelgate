import React, { useState } from 'react';
import { Form, Input, Button, Card, message, Result } from 'antd';
import { UserOutlined, LockOutlined, MailOutlined } from '@ant-design/icons';
import { useNavigate, Link } from 'react-router-dom';
import api from '../api';

const Register: React.FC = () => {
  const [loading, setLoading] = useState(false);
  const [registered, setRegistered] = useState(false);
  const navigate = useNavigate();
  const [messageApi, contextHolder] = message.useMessage();

  const onFinish = async (values: { email: string; password: string; name: string }) => {
    setLoading(true);
    try {
      await api.post('/api/v1/auth/register', {
        email: values.email,
        password: values.password,
        name: values.name,
      });
      setRegistered(true);
    } catch (err: any) {
      const errMsg = err.response?.data?.error;
      if (errMsg === 'email already exists') {
        messageApi.error('该邮箱已被注册');
      } else {
        messageApi.error(errMsg || '注册失败');
      }
    } finally {
      setLoading(false);
    }
  };

  if (registered) {
    return (
      <div style={{
        height: '100vh',
        display: 'flex',
        justifyContent: 'center',
        alignItems: 'center',
        background: '#f0f2f5'
      }}>
        <Card style={{ width: 450 }}>
          <Result
            status="success"
            title="注册成功"
            subTitle="您的账号已创建，请等待管理员审核通过后方可登录使用。"
            extra={
              <Button type="primary" onClick={() => navigate('/login')}>
                返回登录
              </Button>
            }
          />
        </Card>
      </div>
    );
  }

  return (
    <div style={{
      height: '100vh',
      display: 'flex',
      justifyContent: 'center',
      alignItems: 'center',
      background: '#f0f2f5'
    }}>
      <Card title="模界（Model Gate）注册" style={{ width: 400 }}>
        {contextHolder}
        <Form onFinish={onFinish}>
          <Form.Item
            name="name"
            rules={[{ required: true, message: '请输入姓名' }]}
          >
            <Input prefix={<UserOutlined />} placeholder="姓名" />
          </Form.Item>
          <Form.Item
            name="email"
            rules={[
              { required: true, message: '请输入邮箱' },
              { type: 'email', message: '请输入有效的邮箱地址' },
            ]}
          >
            <Input prefix={<MailOutlined />} placeholder="邮箱" />
          </Form.Item>
          <Form.Item
            name="password"
            rules={[
              { required: true, message: '请输入密码' },
              { min: 6, message: '密码至少6位' },
            ]}
          >
            <Input.Password prefix={<LockOutlined />} placeholder="密码" />
          </Form.Item>
          <Form.Item
            name="confirmPassword"
            dependencies={['password']}
            rules={[
              { required: true, message: '请确认密码' },
              ({ getFieldValue }) => ({
                validator(_, value) {
                  if (!value || getFieldValue('password') === value) {
                    return Promise.resolve();
                  }
                  return Promise.reject(new Error('两次输入的密码不一致'));
                },
              }),
            ]}
          >
            <Input.Password prefix={<LockOutlined />} placeholder="确认密码" />
          </Form.Item>
          <Form.Item>
            <Button type="primary" htmlType="submit" loading={loading} block>
              注册
            </Button>
          </Form.Item>
          <div style={{ textAlign: 'center' }}>
            已有账号？<Link to="/login">立即登录</Link>
          </div>
        </Form>
      </Card>
    </div>
  );
};

export default Register;
