import React, { useEffect, useState } from 'react';
import { Card, Button, Form, Input, Row, Col, Switch, message, Tabs, Divider } from 'antd';
import {
  ReloadOutlined,
  SaveOutlined,
  SettingOutlined,
  HourglassOutlined,
  UserOutlined,
  DesktopOutlined,
  SafetyCertificateOutlined,
  DatabaseOutlined
} from '@ant-design/icons';
import api from '../../api';

const SystemTab: React.FC = () => {
  const [form] = Form.useForm();
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [ssoEnabled, setSsoEnabled] = useState(false);
  const [stats, setStats] = useState({
    usersCount: 0,
    modelsCount: 0,
    policiesCount: 0,
    backendsCount: 0,
  });

  const [messageApi, contextHolder] = message.useMessage();

  const fetchConfig = async () => {
    try {
      const [sysRes, pubRes] = await Promise.all([
        api.get('/api/v1/admin/config/system'),
        api.get('/api/v1/config/frontend'),
      ]);
      const data = sysRes.data.data;
      form.setFieldsValue({
        frontend: {
          feedback_url: data.frontend.feedback_url,
          dev_manual_url: data.frontend.dev_manual_url,
          registration_enabled: data.frontend.registration_enabled,
        },
        server: {
          read_timeout: data.server.read_timeout,
          write_timeout: data.server.write_timeout,
          idle_timeout: data.server.idle_timeout,
        }
      });
      setSsoEnabled(pubRes.data.data.sso_enabled || false);
    } catch {
      messageApi.error('获取系统配置失败');
    }
  };

  const fetchStats = async () => {
    try {
      const [usersRes, modelsRes, policiesRes] = await Promise.all([
        api.get('/api/v1/admin/users', { params: { page: 1, page_size: 1 } }),
        api.get('/api/v1/admin/models'),
        api.get('/api/v1/admin/policies'),
      ]);

      const modelsList = modelsRes.data.data || [];
      let totalBackends = 0;

      // Fetch backends for each model in parallel to sum up backends count
      await Promise.all(
        modelsList.map(async (model: { id: string }) => {
          try {
            const backendsRes = await api.get(`/api/v1/admin/models/${encodeURIComponent(model.id)}/backends`);
            totalBackends += (backendsRes.data.data || []).length;
          } catch {
            // Ignore single errors
          }
        })
      );

      setStats({
        usersCount: usersRes.data.total || 0,
        modelsCount: modelsList.length,
        policiesCount: (policiesRes.data.data || []).length,
        backendsCount: totalBackends,
      });
    } catch {
      messageApi.error('获取统计数据失败');
    }
  };

  const handleSaveAll = async (values: any) => {
    setSaving(true);
    try {
      const payload = {
        server: {
          read_timeout: values?.server?.read_timeout,
          write_timeout: values?.server?.write_timeout,
          idle_timeout: values?.server?.idle_timeout,
        },
        frontend: {
          feedback_url: values?.frontend?.feedback_url || '',
          dev_manual_url: values?.frontend?.dev_manual_url || '',
          registration_enabled: values?.frontend?.registration_enabled || false,
        }
      };

      await api.put('/api/v1/admin/config/system', payload);
      messageApi.success('系统配置与核心超时设置保存成功，已动态生效！');
      await fetchConfig();
    } catch (err: unknown) {
      const error = err as { response?: { data?: { error?: string } } };
      messageApi.error(error.response?.data?.error || '保存系统配置失败');
    } finally {
      setSaving(false);
    }
  };

  const loadAllData = async () => {
    setLoading(true);
    await Promise.all([
      fetchConfig(),
      fetchStats(),
    ]);
    setLoading(false);
  };

  useEffect(() => {
    loadAllData();
  }, []);

  const validateDuration = (_: any, value: string) => {
    if (!value) {
      return Promise.reject(new Error('请输入超时时长'));
    }
    const durationRegex = /^(\d+(ns|us|µs|ms|s|m|h))+$/;
    if (!durationRegex.test(value)) {
      return Promise.reject(new Error('请输入有效的超时时长格式（例如: 60s, 30m, 300s）'));
    }
    return Promise.resolve();
  };

  const tabItems = [
    {
      key: 'frontend',
      forceRender: true,
      label: (
        <span>
          <SettingOutlined /> 前端与基础配置
        </span>
      ),
      children: (
        <div style={{ padding: '8px 16px' }}>
          <Divider titlePlacement="left" style={{ margin: '0 0 20px 0', fontSize: '15px', color: '#1f1f1f' }}>前端链接与交互</Divider>
          <Form.Item
            name={['frontend', 'feedback_url']}
            label="用户反馈链接"
            rules={[{ type: 'url', message: '请输入有效的URL（例如: https://example.com）' }]}
            extra="将在前端导航或帮助菜单中展示，方便用户反馈问题。"
          >
            <Input placeholder="如：https://feedback.example.com" size="large" />
          </Form.Item>

          <Form.Item
            name={['frontend', 'dev_manual_url']}
            label="开发者手册链接"
            rules={[{ type: 'url', message: '请输入有效的URL（例如: https://example.com）' }]}
            extra="提供给接入人员的 API 参考或系统使用说明文档。"
          >
            <Input placeholder="如：https://docs.example.com" size="large" />
          </Form.Item>

          <Divider titlePlacement="left" style={{ margin: '30px 0 20px 0', fontSize: '15px', color: '#1f1f1f' }}>安全与自助功能</Divider>
          <Form.Item
            name={['frontend', 'registration_enabled']}
            label="开放自助注册"
            valuePropName="checked"
            extra="开启后，登录页将显示注册按钮。注册后的账号为‘待审核’状态，需管理员审核启用后方可使用。"
          >
            <Switch checkedChildren="已开放" unCheckedChildren="已关闭" />
          </Form.Item>

          <Form.Item label="SSO 单点登录状态" style={{ marginTop: '24px' }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
              <Switch checked={ssoEnabled} disabled checkedChildren="启用" unCheckedChildren="禁用" />
              <span style={{ fontSize: '13px', color: '#8c8c8c' }}>
                {ssoEnabled ? '已启用（SSO 状态由服务器配置文件决定，在此仅作展示）' : '未启用（可在 server.sso 中配置启用）'}
              </span>
            </div>
          </Form.Item>
        </div>
      )
    },
    {
      key: 'timeouts',
      forceRender: true,
      label: (
        <span>
          <HourglassOutlined /> 服务核心超时配置
        </span>
      ),
      children: (
        <div style={{ padding: '8px 16px' }}>
          <Divider titlePlacement="left" style={{ margin: '0 0 20px 0', fontSize: '15px', color: '#1f1f1f' }}>网络与响应超时设置</Divider>
          
          <Form.Item
            name={['server', 'read_timeout']}
            label="读取请求超时 (Read Timeout)"
            rules={[{ required: true, message: '请输入读取请求超时时长' }, { validator: validateDuration }]}
            extra="服务器读取完整请求体（包含流式上传等数据）的最大允许时间。如：60s"
          >
            <Input placeholder="如：60s" size="large" />
          </Form.Item>

          <Form.Item
            name={['server', 'write_timeout']}
            label="写入响应超时 (Write Timeout)"
            rules={[{ required: true, message: '请输入写入响应超时时长' }, { validator: validateDuration }]}
            extra="服务器向客户端写入响应数据的最大允许时间。为支持超长大模型流式输出，建议设为较长值（如 30m）。"
          >
            <Input placeholder="如：30m" size="large" />
          </Form.Item>

          <Form.Item
            name={['server', 'idle_timeout']}
            label="连接空闲超时 (Idle Timeout)"
            rules={[{ required: true, message: '请输入连接空闲超时时长' }, { validator: validateDuration }]}
            extra="当启用 Keep-Alive 时，两个连续请求之间的最长等待时间。如：300s"
          >
            <Input placeholder="如：300s" size="large" />
          </Form.Item>
        </div>
      )
    }
  ];

  const statsData = [
    {
      title: '总用户数',
      value: stats.usersCount,
      gradient: 'linear-gradient(135deg, #e6f7ff 0%, #bae7ff 100%)',
      icon: <UserOutlined style={{ fontSize: '24px', color: '#1890ff' }} />,
    },
    {
      title: '当前模型数',
      value: stats.modelsCount,
      gradient: 'linear-gradient(135deg, #f6ffed 0%, #d9f7be 100%)',
      icon: <DesktopOutlined style={{ fontSize: '24px', color: '#52c41a' }} />,
    },
    {
      title: '配额策略数',
      value: stats.policiesCount,
      gradient: 'linear-gradient(135deg, #f9f0ff 0%, #efdbff 100%)',
      icon: <SafetyCertificateOutlined style={{ fontSize: '24px', color: '#722ed1' }} />,
    },
    {
      title: '后端实例数',
      value: stats.backendsCount,
      gradient: 'linear-gradient(135deg, #fff7e6 0%, #ffe7ba 100%)',
      icon: <DatabaseOutlined style={{ fontSize: '24px', color: '#fa8c16' }} />,
    },
  ];

  return (
    <div style={{ padding: '8px' }}>
      {contextHolder}
      <style>{`
        .premium-stat-card {
          transition: all 0.3s cubic-bezier(0.25, 0.8, 0.25, 1);
          border: 1px solid rgba(0, 0, 0, 0.05);
          border-radius: 12px;
          background: linear-gradient(135deg, #ffffff 0%, #fcfdfe 100%);
          box-shadow: 0 4px 10px rgba(0, 0, 0, 0.02);
        }
        .premium-stat-card:hover {
          transform: translateY(-4px);
          box-shadow: 0 12px 20px rgba(0, 0, 0, 0.08);
          border-color: rgba(24, 144, 255, 0.3);
        }
        .premium-config-card {
          border-radius: 16px;
          box-shadow: 0 8px 30px rgba(0, 0, 0, 0.04);
          border: 1px solid rgba(0, 0, 0, 0.05);
          background: #ffffff;
        }
        .ant-tabs-left > .ant-tabs-nav {
          border-right: 1px solid rgba(0, 0, 0, 0.06);
        }
        .ant-tabs-left > .ant-tabs-nav .ant-tabs-tab {
          padding: 14px 20px;
          margin: 4px 0;
          border-radius: 6px;
          transition: all 0.2s;
        }
        .ant-tabs-left > .ant-tabs-nav .ant-tabs-tab-active {
          background-color: #f0f7ff;
        }
        .config-footer {
          margin-top: 24px;
          padding: 16px 24px;
          background: #fafafa;
          border-top: 1px solid #f0f0f0;
          border-radius: 0 0 16px 16px;
          display: flex;
          justify-content: flex-end;
          gap: 12px;
        }
      `}</style>

      <Row gutter={24}>
        <Col span={16}>
          <Card
            className="premium-config-card"
            title={
              <span style={{ fontSize: '18px', fontWeight: 600, color: '#1f1f1f' }}>
                系统通用与核心参数配置
              </span>
            }
            styles={{ body: { padding: '24px 0 0 0' } }}
            loading={loading}
          >
            <Form
              form={form}
              layout="vertical"
              onFinish={handleSaveAll}
            >
              <div style={{ minHeight: '400px' }}>
                <Tabs
                  tabPosition="left"
                  items={tabItems}
                  defaultActiveKey="frontend"
                  style={{ width: '100%' }}
                />
              </div>

              <div className="config-footer">
                <Button
                  icon={<ReloadOutlined />}
                  onClick={fetchConfig}
                  size="large"
                >
                  放弃修改并刷新
                </Button>
                <Button
                  type="primary"
                  icon={<SaveOutlined />}
                  onClick={() => form.submit()}
                  loading={saving}
                  size="large"
                >
                  保存所有配置并生效
                </Button>
              </div>
            </Form>
          </Card>
        </Col>

        <Col span={8}>
          <Card
            className="premium-config-card"
            title={
              <span style={{ fontSize: '18px', fontWeight: 600, color: '#1f1f1f' }}>
                系统数据实时统计
              </span>
            }
            styles={{ body: { padding: '24px' } }}
            extra={
              <Button
                type="text"
                shape="circle"
                icon={<ReloadOutlined style={{ color: '#8c8c8c' }} />}
                onClick={fetchStats}
              />
            }
          >
            <Row gutter={[16, 16]}>
              {statsData.map((item, index) => (
                <Col span={24} key={index}>
                  <Card className="premium-stat-card" styles={{ body: { padding: '18px 24px' } }}>
                    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                      <div>
                        <div style={{ color: '#8c8c8c', fontSize: '13px', marginBottom: '4px' }}>
                          {item.title}
                        </div>
                        <div style={{
                          fontSize: '28px',
                          fontWeight: 700,
                          color: '#262626',
                          fontFamily: '-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial'
                        }}>
                          {item.value}
                        </div>
                      </div>
                      <div style={{
                        width: '48px',
                        height: '48px',
                        borderRadius: '10px',
                        background: item.gradient,
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'center'
                      }}>
                        {item.icon}
                      </div>
                    </div>
                  </Card>
                </Col>
              ))}
            </Row>
          </Card>
        </Col>
      </Row>
    </div>
  );
};

export default SystemTab;
