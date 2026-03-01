import React, { useEffect, useState } from 'react';
import { Layout, Card, Table, Button, Tag, message, Tabs, Modal, Form, Input, Space, Popconfirm, Statistic, Row, Col, Tooltip, Switch } from 'antd';
import { ArrowLeftOutlined, PlusOutlined, EditOutlined, DeleteOutlined, SettingOutlined, ReloadOutlined, CheckCircleOutlined, CloseCircleOutlined } from '@ant-design/icons';
import { useNavigate } from 'react-router-dom';
import api from '../api';

const { Header, Content } = Layout;

interface Model {
  id: string;
  name: string;
  description: string;
  enabled: boolean;
  model_params?: Record<string, any>;
  created_at: string;
  updated_at: string;
  backend_count?: number;
}

interface Backend {
  id: string;
  model_id: string;
  name: string;
  base_url: string;
  model_name: string;
  weight: number;
  region: string;
  enabled: boolean;
  healthy: boolean;
  last_check_at: string;
  created_at: string;
  updated_at: string;
}

interface BackendHealth {
  backend_id: string;
  url: string;
  model_name: string;
  healthy: boolean;
  last_check: string;
  fail_count: number;
  latency_ms: number;
}

interface User {
  id: string;
  email: string;
  name: string;
  role: string;
  department: string;
  quota_policy: string;
}

interface Policy {
  name: string;
  rate_limit: number;
  token_quota_daily: number;
  token_quota_monthly: number;
}

const Admin: React.FC = () => {
  const [users, setUsers] = useState<User[]>([]);
  const [models, setModels] = useState<Model[]>([]);
  const [policies, setPolicies] = useState<Policy[]>([]);
  const [backends, setBackends] = useState<Backend[]>([]);
  const [healthStatus, setHealthStatus] = useState<Record<string, BackendHealth>>({});
  const [activeTab, setActiveTab] = useState('users');
  const [loading, setLoading] = useState(false);
  const navigate = useNavigate();

  // Modal states
  const [modelModalVisible, setModelModalVisible] = useState(false);
  const [modelModalTitle, setModelModalTitle] = useState('创建模型');
  const [editingModel, setEditingModel] = useState<Model | null>(null);
  const [modelForm] = Form.useForm();

  const [messageApi, contextHolder] = message.useMessage();

  const fetchData = async () => {
    setLoading(true);
    try {
      const [usersRes, modelsRes, policiesRes] = await Promise.all([
        api.get('/api/v1/admin/users'),
        api.get('/api/v1/admin/models'),
        api.get('/api/v1/admin/policies'),
      ]);
      setUsers(usersRes.data.data || []);
      setPolicies(policiesRes.data.data || []);

      // Fetch backend counts for each model
      const modelsData = modelsRes.data.data || [];
      const modelsWithBackendCount = await Promise.all(
        modelsData.map(async (model: Model) => {
          try {
            const backendsRes = await api.get(`/api/v1/admin/models/${model.id}/backends`);
            const backendsList = backendsRes.data.data || [];
            return { ...model, backend_count: backendsList.length };
          } catch {
            return { ...model, backend_count: 0 };
          }
        })
      );
      setModels(modelsWithBackendCount);
    } catch {
      messageApi.error('获取数据失败');
    } finally {
      setLoading(false);
    }
  };

  const fetchHealthStatus = async () => {
    try {
      const res = await api.get('/api/v1/admin/models/health');
      setHealthStatus(res.data.data || {});
    } catch {
      messageApi.error('获取健康状态失败');
    }
  };

  const fetchAllBackends = async () => {
    try {
      const allBackends: Backend[] = [];
      for (const model of models) {
        try {
          const res = await api.get(`/api/v1/admin/models/${model.id}/backends`);
          const modelBackends = (res.data.data || []).map((b: Backend) => ({
            ...b,
            model_id: model.id,
            model_name: model.name,
          }));
          allBackends.push(...modelBackends);
        } catch {
          // Skip failed requests
        }
      }
      setBackends(allBackends);
    } catch {
      messageApi.error('获取后端列表失败');
    }
  };

  useEffect(() => {
    fetchData();
  }, []);

  useEffect(() => {
    if (activeTab === 'health') {
      fetchHealthStatus();
      fetchAllBackends();
      // Auto refresh every 30 seconds
      const interval = setInterval(() => {
        fetchHealthStatus();
      }, 30000);
      return () => clearInterval(interval);
    }
  }, [activeTab, models]);

  // Model management functions
  const handleCreateModel = () => {
    setEditingModel(null);
    setModelModalTitle('创建模型');
    modelForm.resetFields();
    modelForm.setFieldsValue({ enabled: true });
    setModelModalVisible(true);
  };

  const handleEditModel = (model: Model) => {
    setEditingModel(model);
    setModelModalTitle('编辑模型');
    modelForm.setFieldsValue({
      id: model.id,
      name: model.name,
      description: model.description,
      enabled: model.enabled,
      model_params: model.model_params ? JSON.stringify(model.model_params, null, 2) : '',
    });
    setModelModalVisible(true);
  };

interface ModelFormValues {
  id: string;
  name: string;
  description: string;
  enabled: boolean;
  model_params?: string;
}

  const handleModelSubmit = async (values: ModelFormValues) => {
    try {
      // 解析 model_params JSON
      let modelParams = undefined;
      if (values.model_params) {
        try {
          modelParams = JSON.parse(values.model_params);
        } catch {
          messageApi.error('模型参数 JSON 格式错误');
          return;
        }
      }

      if (editingModel) {
        // Update existing model
        await api.put(`/api/v1/admin/models/${editingModel.id}`, {
          name: values.name,
          description: values.description,
          enabled: values.enabled,
          model_params: modelParams,
        });
        messageApi.success('模型更新成功');
      } else {
        // Create new model
        await api.post('/api/v1/admin/models', {
          id: values.id,
          name: values.name,
          description: values.description,
          enabled: values.enabled,
          model_params: modelParams,
        });
        messageApi.success('模型创建成功');
      }
      setModelModalVisible(false);
      fetchData();
    } catch (err: unknown) {
      const error = err as { response?: { data?: { error?: string } } };
      messageApi.error(error.response?.data?.error || '操作失败');
    }
  };

  const handleDeleteModel = async (model: Model) => {
    try {
      await api.delete(`/api/v1/admin/models/${model.id}`);
      messageApi.success('模型删除成功');
      fetchData();
    } catch (err: unknown) {
      const error = err as { response?: { data?: { error?: string } } };
      messageApi.error(error.response?.data?.error || '删除失败');
    }
  };

  const handleToggleModelEnabled = async (model: Model) => {
    try {
      await api.put(`/api/v1/admin/models/${model.id}`, {
        name: model.name,
        description: model.description,
        enabled: !model.enabled,
      });
      messageApi.success(model.enabled ? '模型已禁用' : '模型已启用');
      fetchData();
    } catch (err: unknown) {
      const error = err as { response?: { data?: { error?: string } } };
      messageApi.error(error.response?.data?.error || '操作失败');
    }
  };

  const handleManageBackends = (modelId: string) => {
    navigate(`/admin/models/${modelId}/backends`);
  };

  const userColumns = [
    { title: '邮箱', dataIndex: 'email' },
    { title: '姓名', dataIndex: 'name' },
    { title: '角色', dataIndex: 'role' },
    { title: '部门', dataIndex: 'department' },
    {
      title: '配额策略',
      dataIndex: 'quota_policy',
      render: (v: string) => <Tag>{v}</Tag>,
    },
    {
      title: '操作',
      render: () => (
        <Button size="small" icon={<EditOutlined />}>
          编辑
        </Button>
      ),
    },
  ];

  const modelColumns = [
    { title: 'ID', dataIndex: 'id', ellipsis: true },
    { title: '名称', dataIndex: 'name' },
    { title: '描述', dataIndex: 'description', ellipsis: true },
    {
      title: '后端数量',
      dataIndex: 'backend_count',
      render: (count: number) => <Tag color="blue">{count || 0}</Tag>,
    },
    {
      title: '状态',
      dataIndex: 'enabled',
      render: (enabled: boolean, record: Model) => (
        <Tag
          color={enabled ? 'green' : 'default'}
          style={{ cursor: 'pointer' }}
          onClick={() => handleToggleModelEnabled(record)}
        >
          {enabled ? '启用' : '禁用'}
        </Tag>
      ),
    },
    {
      title: '操作',
      render: (_: unknown, record: Model) => (
        <Space>
          <Tooltip title="编辑">
            <Button
              type="link"
              size="small"
              icon={<EditOutlined />}
              onClick={() => handleEditModel(record)}
            />
          </Tooltip>
          <Tooltip title="管理后端">
            <Button
              type="link"
              size="small"
              icon={<SettingOutlined />}
              onClick={() => handleManageBackends(record.id)}
            />
          </Tooltip>
          <Popconfirm
            title="确认删除"
            description={`删除模型 "${record.name}" 将同时删除其所有后端实例，确定要继续吗？`}
            onConfirm={() => handleDeleteModel(record)}
            okText="删除"
            cancelText="取消"
            okButtonProps={{ danger: true }}
          >
            <Tooltip title="删除">
              <Button type="link" danger size="small" icon={<DeleteOutlined />} />
            </Tooltip>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  const policyColumns = [
    { title: '名称', dataIndex: 'name' },
    { title: '速率限制', dataIndex: 'rate_limit', render: (v: number) => `${v}/min` },
    { title: 'Token 日限额', dataIndex: 'token_quota_daily' },
    { title: 'Token 月限额', dataIndex: 'token_quota_monthly' },
  ];

  const healthColumns = [
    {
      title: '后端ID',
      dataIndex: 'id',
      key: 'id',
      ellipsis: true,
    },
    {
      title: '所属模型',
      dataIndex: 'model_id',
      key: 'model_id',
      render: (modelId: string) => {
        const model = models.find(m => m.id === modelId);
        return model ? model.name : modelId;
      },
    },
    {
      title: '名称',
      dataIndex: 'name',
      key: 'name',
      render: (name: string) => name || '-',
    },
    {
      title: 'URL',
      dataIndex: 'base_url',
      key: 'base_url',
      ellipsis: true,
    },
    {
      title: '健康状态',
      key: 'health',
      render: (_: unknown, record: Backend) => {
        const health = healthStatus[record.id];
        if (!health) {
          return <Tag color="orange">检查中</Tag>;
        }
        return health.healthy ? (
          <Tag color="green" icon={<CheckCircleOutlined />}>健康</Tag>
        ) : (
          <Tag color="red" icon={<CloseCircleOutlined />}>不健康</Tag>
        );
      },
    },
    {
      title: '延迟',
      key: 'latency',
      render: (_: unknown, record: Backend) => {
        const health = healthStatus[record.id];
        if (!health || health.latency_ms === 0) return '-';
        return `${health.latency_ms}ms`;
      },
    },
    {
      title: '失败次数',
      key: 'fail_count',
      render: (_: unknown, record: Backend) => {
        const health = healthStatus[record.id];
        if (!health || health.fail_count === 0) return '-';
        return <Tag color="red">{health.fail_count}</Tag>;
      },
    },
    {
      title: '最后检查',
      key: 'last_check',
      render: (_: unknown, record: Backend) => {
        const health = healthStatus[record.id];
        if (!health || !health.last_check) return '-';
        return new Date(health.last_check).toLocaleString();
      },
    },
  ];

  const healthyCount = Object.values(healthStatus).filter(h => h.healthy).length;
  const unhealthyCount = Object.values(healthStatus).filter(h => !h.healthy).length;
  const totalBackends = Object.keys(healthStatus).length;

  return (
    <Layout style={{ minHeight: '100vh' }}>
      <Header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <div style={{ display: 'flex', alignItems: 'center' }}>
          <Button
            icon={<ArrowLeftOutlined />}
            onClick={() => navigate('/dashboard')}
            style={{ marginRight: 16 }}
          >
            返回
          </Button>
          <h2 style={{ color: '#fff', margin: 0 }}>管理后台</h2>
        </div>
      </Header>
      <Content style={{ padding: 24 }}>
        {contextHolder}
        <Card>
          <Tabs activeKey={activeTab} onChange={setActiveTab}>
            <Tabs.TabPane tab="用户管理" key="users">
              <Table
                dataSource={users}
                columns={userColumns}
                rowKey="id"
                loading={loading}
              />
            </Tabs.TabPane>
            <Tabs.TabPane tab="模型管理" key="models">
              <div style={{ marginBottom: 16 }}>
                <Button
                  type="primary"
                  icon={<PlusOutlined />}
                  onClick={handleCreateModel}
                >
                  创建模型
                </Button>
              </div>
              <Table
                dataSource={models}
                columns={modelColumns}
                rowKey="id"
                loading={loading}
              />
            </Tabs.TabPane>
            <Tabs.TabPane tab="配额策略" key="policies">
              <Table
                dataSource={policies}
                columns={policyColumns}
                rowKey="name"
                loading={loading}
              />
            </Tabs.TabPane>
            <Tabs.TabPane tab="健康监控" key="health">
              <Row gutter={16} style={{ marginBottom: 24 }}>
                <Col span={8}>
                  <Card>
                    <Statistic
                      title="总后端数"
                      value={totalBackends}
                      valueStyle={{ color: '#1890ff' }}
                    />
                  </Card>
                </Col>
                <Col span={8}>
                  <Card>
                    <Statistic
                      title="健康后端"
                      value={healthyCount}
                      valueStyle={{ color: '#52c41a' }}
                      prefix={<CheckCircleOutlined />}
                    />
                  </Card>
                </Col>
                <Col span={8}>
                  <Card>
                    <Statistic
                      title="不健康后端"
                      value={unhealthyCount}
                      valueStyle={{ color: unhealthyCount > 0 ? '#cf1322' : '#999' }}
                      prefix={<CloseCircleOutlined />}
                    />
                  </Card>
                </Col>
              </Row>
              <div style={{ marginBottom: 16 }}>
                <Button
                  icon={<ReloadOutlined />}
                  onClick={() => {
                    fetchHealthStatus();
                    fetchAllBackends();
                  }}
                >
                  刷新
                </Button>
              </div>
              <Table
                dataSource={backends}
                columns={healthColumns}
                rowKey="id"
                loading={loading}
              />
            </Tabs.TabPane>
          </Tabs>
        </Card>
      </Content>

      {/* Model Create/Edit Modal */}
      <Modal
        title={modelModalTitle}
        open={modelModalVisible}
        onCancel={() => setModelModalVisible(false)}
        onOk={() => modelForm.submit()}
        okText={editingModel ? '保存' : '创建'}
        width={600}
      >
        <Form
          form={modelForm}
          onFinish={handleModelSubmit}
          layout="horizontal"
          labelCol={{ span: 6 }}
          wrapperCol={{ span: 18 }}
          style={{ marginTop: 24 }}
        >
          <Form.Item
            name="id"
            label="模型ID"
            rules={[{ required: true, message: '请输入模型ID' }]}
            extra="唯一标识，如：gpt-4, claude-3-opus"
          >
            <Input disabled={!!editingModel} placeholder="如：gpt-4" />
          </Form.Item>

          <Form.Item
            name="name"
            label="显示名称"
            rules={[{ required: true, message: '请输入显示名称' }]}
          >
            <Input placeholder="如：GPT-4" />
          </Form.Item>

          <Form.Item
            name="description"
            label="描述"
          >
            <Input.TextArea rows={3} placeholder="模型描述信息（可选）" />
          </Form.Item>

          <Form.Item
            name="enabled"
            label="启用状态"
            valuePropName="checked"
          >
            <Switch />
          </Form.Item>

          <Form.Item
            name="model_params"
            label="模型参数"
            extra="JSON 格式，如：{&quot;enable_thinking&quot;: false}"
          >
            <Input.TextArea
              rows={4}
              placeholder={`{
  "enable_thinking": false,
  "max_tokens": 4096
}`}
            />
          </Form.Item>
        </Form>
      </Modal>
    </Layout>
  );
};

export default Admin;
