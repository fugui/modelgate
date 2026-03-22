import React, { useState } from 'react';
import { Form, Input, Button, message, Typography, Result } from 'antd';
import { UserOutlined, LockOutlined, MailOutlined } from '@ant-design/icons';
import { useNavigate, Link } from 'react-router-dom';
import api from '../api';
import logo from '../assets/modelgate.png';

const { Title, Text, Paragraph } = Typography;

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

  // 注册成功提示页
  if (registered) {
    return (
      <div style={{
        height: '100vh',
        display: 'flex',
        justifyContent: 'center',
        alignItems: 'center',
        background: 'linear-gradient(135deg, #f5f7fa 0%, #e8ecf1 100%)',
      }}>
        <div style={{
          background: '#fff',
          borderRadius: '16px',
          padding: '48px',
          maxWidth: '460px',
          width: '100%',
          boxShadow: '0 20px 60px rgba(0,0,0,0.08)',
          textAlign: 'center',
        }}>
          <Result
            status="success"
            title="注册成功"
            subTitle="您的账号已创建，请等待管理员审核通过后方可登录使用。"
            extra={
              <Button
                type="primary"
                onClick={() => navigate('/login')}
                style={{
                  height: '44px',
                  borderRadius: '8px',
                  fontWeight: 500,
                  fontSize: '15px',
                  background: 'linear-gradient(135deg, #667eea 0%, #764ba2 100%)',
                  border: 'none',
                  paddingInline: '40px',
                }}
              >
                返回登录
              </Button>
            }
          />
        </div>
      </div>
    );
  }

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
            加入模界
          </Title>
          <Paragraph style={{
            color: 'rgba(255,255,255,0.5)',
            fontSize: '15px',
            lineHeight: '1.8',
            marginTop: '24px',
          }}>
            注册账号后需管理员审核
            <br />
            审核通过即可使用平台全部功能
          </Paragraph>

          {/* 流程步骤 */}
          <div style={{ marginTop: '48px', textAlign: 'left' }}>
            {[
              { step: '1', text: '填写注册信息' },
              { step: '2', text: '等待管理员审核' },
              { step: '3', text: '审核通过后登录使用' },
            ].map(item => (
              <div key={item.step} style={{
                display: 'flex',
                alignItems: 'center',
                gap: '16px',
                marginBottom: '20px',
              }}>
                <div style={{
                  width: '32px',
                  height: '32px',
                  borderRadius: '50%',
                  background: 'rgba(255,255,255,0.1)',
                  border: '1px solid rgba(255,255,255,0.2)',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  color: 'rgba(255,255,255,0.7)',
                  fontSize: '14px',
                  fontWeight: 600,
                  flexShrink: 0,
                }}>
                  {item.step}
                </div>
                <Text style={{ color: 'rgba(255,255,255,0.6)', fontSize: '14px' }}>
                  {item.text}
                </Text>
              </div>
            ))}
          </div>
        </div>
      </div>

      {/* 右侧注册面板 */}
      <div style={{
        flex: 1,
        display: 'flex',
        flexDirection: 'column',
        justifyContent: 'center',
        alignItems: 'center',
        padding: '60px',
      }}>
        <div style={{ width: '100%', maxWidth: '380px' }}>
          <div style={{ marginBottom: '36px' }}>
            <Title level={3} style={{ marginBottom: '8px', color: '#1a1a2e' }}>
              注册账号
            </Title>
            <Text type="secondary">填写以下信息创建您的账号</Text>
          </div>

          <Form
            onFinish={onFinish}
            size="large"
            layout="vertical"
          >
            <Form.Item
              name="name"
              label="姓名"
              rules={[{ required: true, message: '请输入姓名' }]}
            >
              <Input
                prefix={<UserOutlined style={{ color: '#bfbfbf' }} />}
                placeholder="请输入您的姓名"
                style={{ borderRadius: '8px', height: '44px' }}
              />
            </Form.Item>
            <Form.Item
              name="email"
              label="邮箱"
              rules={[
                { required: true, message: '请输入邮箱' },
                { type: 'email', message: '请输入有效的邮箱地址' },
              ]}
            >
              <Input
                prefix={<MailOutlined style={{ color: '#bfbfbf' }} />}
                placeholder="请输入邮箱地址"
                style={{ borderRadius: '8px', height: '44px' }}
              />
            </Form.Item>
            <Form.Item
              name="password"
              label="密码"
              rules={[
                { required: true, message: '请输入密码' },
                { min: 6, message: '密码至少6位' },
              ]}
            >
              <Input.Password
                prefix={<LockOutlined style={{ color: '#bfbfbf' }} />}
                placeholder="请输入密码（至少6位）"
                style={{ borderRadius: '8px', height: '44px' }}
              />
            </Form.Item>
            <Form.Item
              name="confirmPassword"
              label="确认密码"
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
              <Input.Password
                prefix={<LockOutlined style={{ color: '#bfbfbf' }} />}
                placeholder="请再次输入密码"
                style={{ borderRadius: '8px', height: '44px' }}
              />
            </Form.Item>
            <Form.Item style={{ marginTop: '28px' }}>
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
                注 册
              </Button>
            </Form.Item>
          </Form>

          <div style={{
            textAlign: 'center',
            marginTop: '20px',
            padding: '16px',
            borderRadius: '8px',
            background: '#f8f9ff',
          }}>
            <Text type="secondary">已有账号？</Text>
            <Link to="/login" style={{ marginLeft: '4px', fontWeight: 500 }}>
              立即登录
            </Link>
          </div>
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

export default Register;
