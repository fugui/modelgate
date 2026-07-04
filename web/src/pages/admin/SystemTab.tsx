import React, { useEffect, useState } from 'react';
import { Card, Button, Form, Input, Row, Col, Switch, message, Divider, Table, Space, Popconfirm, Tag, Tooltip } from 'antd';
import {
  ReloadOutlined,
  SaveOutlined,
  SettingOutlined,
  HourglassOutlined,
  UserOutlined,
  DesktopOutlined,
  SafetyCertificateOutlined,
  DatabaseOutlined,
  PlusOutlined,
  DeleteOutlined,
  StopOutlined,
} from '@ant-design/icons';
import api from '../../api';

interface ClientFilterRule {
  name: string;
  pattern: string;
  enabled: boolean;
}

const SystemTab: React.FC = () => {
  const [form] = Form.useForm();
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [stats, setStats] = useState({
    usersCount: 0,
    modelsCount: 0,
    policiesCount: 0,
    backendsCount: 0,
  });
  const [clientRules, setClientRules] = useState<ClientFilterRule[]>([]);
  const [newRuleName, setNewRuleName] = useState('');
  const [newRulePattern, setNewRulePattern] = useState('');

  const [messageApi, contextHolder] = message.useMessage();

  const fetchConfig = async () => {
    try {
      const sysRes = await api.get('/api/v1/admin/config/system');
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
      setClientRules(data.client_filter?.rules || []);
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
        },
        client_filter: {
          rules: clientRules,
        },
      };

      await api.put('/api/v1/admin/config/system', payload);
      messageApi.success('系统配置保存成功，已动态生效！');
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
    await Promise.all([fetchConfig(), fetchStats()]);
    setLoading(false);
  };

  useEffect(() => {
    loadAllData();
  }, []);

  const validateDuration = (_: any, value: string) => {
    if (!value) return Promise.reject(new Error('请输入超时时长'));
    const durationRegex = /^(\d+(ns|us|µs|ms|s|m|h))+$/;
    if (!durationRegex.test(value)) {
      return Promise.reject(new Error('请输入有效的超时时长格式（例如: 60s, 30m, 300s）'));
    }
    return Promise.resolve();
  };

  // ---- Client Filter Rule Helpers ----
  const toggleRule = (index: number) => {
    setClientRules(prev =>
      prev.map((r, i) => (i === index ? { ...r, enabled: !r.enabled } : r))
    );
  };

  const deleteRule = (index: number) => {
    setClientRules(prev => prev.filter((_, i) => i !== index));
  };

  const addRule = () => {
    const name = newRuleName.trim();
    const pattern = newRulePattern.trim();
    if (!name || !pattern) {
      messageApi.warning('请填写规则名称和匹配模式');
      return;
    }
    setClientRules(prev => [...prev, { name, pattern, enabled: true }]);
    setNewRuleName('');
    setNewRulePattern('');
  };

  const ruleColumns = [
    {
      title: '客户端名称',
      dataIndex: 'name',
      key: 'name',
      render: (name: string, _record: ClientFilterRule, index: number) => (
        <span style={{ fontWeight: 500 }}>
          {name}
          {index === 0 && (
            <Tag color="orange" style={{ marginLeft: 8, fontSize: '11px' }}>默认</Tag>
          )}
        </span>
      ),
    },
    {
      title: 'User-Agent 匹配模式',
      dataIndex: 'pattern',
      key: 'pattern',
      render: (pattern: string) => (
        <Tooltip title="不区分大小写的子串匹配">
          <code style={{
            background: '#f5f5f5',
            padding: '2px 8px',
            borderRadius: '4px',
            fontSize: '13px',
            color: '#c41d7f',
          }}>{pattern}</code>
        </Tooltip>
      ),
    },
    {
      title: '封禁状态',
      dataIndex: 'enabled',
      key: 'enabled',
      width: 120,
      render: (enabled: boolean, _record: ClientFilterRule, index: number) => (
        <Switch
          checked={enabled}
          checkedChildren="已封禁"
          unCheckedChildren="已放行"
          onChange={() => toggleRule(index)}
          style={enabled ? { backgroundColor: '#ff4d4f' } : {}}
        />
      ),
    },
    {
      title: '操作',
      key: 'action',
      width: 80,
      render: (_: any, _record: ClientFilterRule, index: number) => (
        <Popconfirm
          title="确认删除此规则？"
          onConfirm={() => deleteRule(index)}
          okText="删除"
          cancelText="取消"
          okType="danger"
        >
          <Button type="text" danger icon={<DeleteOutlined />} size="small" />
        </Popconfirm>
      ),
    },
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
        .premium-block-card {
          border: 1px solid #f0f0f0;
          border-radius: 12px;
          background: #fafafa;
          padding: 24px 24px 12px 24px;
          height: 100%;
          box-shadow: 0 2px 8px rgba(0,0,0,0.01);
          transition: all 0.3s ease;
        }
        .premium-block-card:hover {
          border-color: #e6f7ff;
          background: #fcfdfe;
          box-shadow: 0 4px 16px rgba(24, 144, 255, 0.04);
        }
        .client-filter-card {
          border: 1px solid #fff1f0;
          border-radius: 12px;
          background: #fffbfa;
          padding: 24px 24px 16px 24px;
          box-shadow: 0 2px 8px rgba(255, 77, 79, 0.03);
          transition: all 0.3s ease;
        }
        .client-filter-card:hover {
          border-color: #ffccc7;
          box-shadow: 0 4px 16px rgba(255, 77, 79, 0.06);
        }
        .config-footer {
          margin-top: 24px;
          padding: 16px 24px;
          background: #fafafa;
          border-top: 1px solid #f0f0f0;
          border-radius: 0 0 16px 16px;
          display: flex;
          justify-content: center;
          gap: 16px;
        }
        .add-rule-row {
          display: flex;
          gap: 8px;
          align-items: center;
          margin-top: 12px;
          padding: 12px;
          background: #f9f9f9;
          border-radius: 8px;
          border: 1px dashed #d9d9d9;
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
            styles={{ body: { padding: '24px 24px 0 24px' } }}
            loading={loading}
          >
            <Form form={form} layout="vertical" onFinish={handleSaveAll}>
              <div style={{ padding: '0 8px' }}>
                <Row gutter={32}>
                  {/* Column 1: 前端与基础配置 */}
                  <Col span={12}>
                    <div className="premium-block-card">
                      <Divider titlePlacement="left" style={{ margin: '0 0 24px 0' }}>
                        <span style={{ fontSize: '15px', fontWeight: 600, color: '#1890ff' }}>
                          <SettingOutlined style={{ marginRight: '8px' }} /> 前端与基础配置
                        </span>
                      </Divider>

                      <Form.Item
                        name={['frontend', 'feedback_url']}
                        label="用户反馈链接"
                        rules={[{ type: 'url', message: '请输入有效的URL（例如: https://example.com）' }]}
                        extra="将在前端导航或帮助菜单中展示，方便用户反馈。"
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

                      <Form.Item
                        name={['frontend', 'registration_enabled']}
                        label="开放自助注册"
                        valuePropName="checked"
                        extra="开启后登录页显示注册。账号需管理员审核后方可使用。"
                      >
                        <Switch checkedChildren="已开放" unCheckedChildren="已关闭" />
                      </Form.Item>
                    </div>
                  </Col>

                  {/* Column 2: 服务核心超时配置 */}
                  <Col span={12}>
                    <div className="premium-block-card">
                      <Divider titlePlacement="left" style={{ margin: '0 0 24px 0' }}>
                        <span style={{ fontSize: '15px', fontWeight: 600, color: '#1890ff' }}>
                          <HourglassOutlined style={{ marginRight: '8px' }} /> 服务核心超时配置
                        </span>
                      </Divider>

                      <Form.Item
                        name={['server', 'read_timeout']}
                        label="读取请求超时 (Read Timeout)"
                        rules={[
                          { required: true, message: '请输入读取请求超时时长' },
                          { validator: validateDuration },
                        ]}
                        extra="服务器读取完整请求体（包含流式上传等）的最大允许时间。如：60s"
                      >
                        <Input placeholder="如：60s" size="large" />
                      </Form.Item>

                      <Form.Item
                        name={['server', 'write_timeout']}
                        label="写入响应超时 (Write Timeout)"
                        rules={[
                          { required: true, message: '请输入写入响应超时时长' },
                          { validator: validateDuration },
                        ]}
                        extra="服务器向客户端写入响应的最大允许时间。流式输出建议设为较长值（如 30m）。"
                      >
                        <Input placeholder="如：30m" size="large" />
                      </Form.Item>

                      <Form.Item
                        name={['server', 'idle_timeout']}
                        label="连接空闲超时 (Idle Timeout)"
                        rules={[
                          { required: true, message: '请输入连接空闲超时时长' },
                          { validator: validateDuration },
                        ]}
                        extra="启用 Keep-Alive 时，两个连续请求之间的最长等待时间。如：300s"
                      >
                        <Input placeholder="如：300s" size="large" />
                      </Form.Item>
                    </div>
                  </Col>
                </Row>

                {/* 客户端访问控制 */}
                <div className="client-filter-card" style={{ marginTop: 24 }}>
                  <Divider titlePlacement="left" style={{ margin: '0 0 20px 0' }}>
                    <span style={{ fontSize: '15px', fontWeight: 600, color: '#cf1322' }}>
                      <StopOutlined style={{ marginRight: '8px' }} /> 客户端访问控制
                    </span>
                  </Divider>
                  <p style={{ color: '#8c8c8c', fontSize: '13px', marginBottom: 16, marginTop: -8 }}>
                    根据请求的 <code>User-Agent</code> 封禁特定客户端类型。封禁状态开启时，匹配该模式的请求将被拒绝（HTTP 403）。
                  </p>
                  <Table
                    dataSource={clientRules}
                    columns={ruleColumns}
                    rowKey={(_record, index) => String(index)}
                    pagination={false}
                    size="small"
                    style={{ marginBottom: 0 }}
                    locale={{ emptyText: '暂无封禁规则，所有客户端均可访问' }}
                  />
                  {/* 添加新规则 */}
                  <div className="add-rule-row">
                    <Input
                      placeholder="规则名称（如：My Bot）"
                      value={newRuleName}
                      onChange={e => setNewRuleName(e.target.value)}
                      style={{ flex: 1 }}
                      onPressEnter={addRule}
                    />
                    <Input
                      placeholder="UA 匹配模式（如：my-bot）"
                      value={newRulePattern}
                      onChange={e => setNewRulePattern(e.target.value)}
                      style={{ flex: 1 }}
                      onPressEnter={addRule}
                    />
                    <Space>
                      <Button
                        type="primary"
                        icon={<PlusOutlined />}
                        onClick={addRule}
                        style={{ background: '#cf1322', borderColor: '#cf1322' }}
                      >
                        添加封禁规则
                      </Button>
                    </Space>
                  </div>
                </div>
              </div>

              <div className="config-footer">
                <Button icon={<ReloadOutlined />} onClick={fetchConfig} size="large">
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
                          fontFamily: '-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial',
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
                        justifyContent: 'center',
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
