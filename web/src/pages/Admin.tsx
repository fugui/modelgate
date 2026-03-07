import React, { useEffect, useState } from 'react';
import { Card, Table, Button, Tag, message, Tabs, Modal, Form, Input, Space, Popconfirm, Statistic, Row, Col, Tooltip, Switch, Drawer, InputNumber } from 'antd';
import { PlusOutlined, EditOutlined, DeleteOutlined, SettingOutlined, ReloadOutlined, CheckCircleOutlined, CloseCircleOutlined, ArrowLeftOutlined } from '@ant-design/icons';
import { useParams, useNavigate } from 'react-router-dom';
import api from '../api';

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
}

interface Policy {
  name: string;
  rate_limit: number;
  request_quota_daily: number;
}

interface ModelFormValues {
  id: string;
  name: string;
  description: string;
  enabled: boolean;
  model_params?: string;
}

interface BackendFormValues {
  id: string;
  name: string;
  base_url: string;
  model_name: string;
  api_key: string;
  weight: number;
  region: string;
  enabled: boolean;
}

const Admin: React.FC = () => {
  const { tab } = useParams<{ tab?: string }>();
  const navigate = useNavigate();

  const [users, setUsers] = useState<User[]>([]);
  const [models, setModels] = useState<Model[]>([]);
  const [policies, setPolicies] = useState<Policy[]>([]);
  const [backends, setBackends] = useState<Backend[]>([]);
  const [healthStatus, setHealthStatus] = useState<Record<string, BackendHealth>>({});
  const [loading, setLoading] = useState(false);

  // Backend drawer states
  const [backendDrawerVisible, setBackendDrawerVisible] = useState(false);
  const [selectedModelId, setSelectedModelId] = useState<string | null>(null);
  const [selectedModelName, setSelectedModelName] = useState<string>('');
  const [modelBackends, setModelBackends] = useState<Backend[]>([]);
  const [backendsLoading, setBackendsLoading] = useState(false);

  // Backend modal states
  const [backendModalVisible, setBackendModalVisible] = useState(false);
  const [backendModalTitle, setBackendModalTitle] = useState('创建后端');
  const [editingBackend, setEditingBackend] = useState<Backend | null>(null);
  const [backendForm] = Form.useForm();

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

  // Validate and set active tab from URL
  useEffect(() => {
    const validTabs = ['users', 'models', 'policies', 'health'];
    const currentTab = tab || 'users';
    if (!validTabs.includes(currentTab)) {
      navigate('/admin/users', { replace: true });
    }
  }, [tab, navigate]);

  useEffect(() => {
    fetchData();
  }, []);

  useEffect(() => {
    const currentTab = tab || 'users';
    if (currentTab === 'health') {
      fetchHealthStatus();
      fetchAllBackends();
      // Auto refresh every 30 seconds
      const interval = setInterval(() => {
        fetchHealthStatus();
      }, 30000);
      return () => clearInterval(interval);
    }
  }, [tab, models]);

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

  const handleManageBackends = async (modelId: string, modelName: string) => {
    setSelectedModelId(modelId);
    setSelectedModelName(modelName);
    setBackendDrawerVisible(true);
    await fetchModelBackends(modelId);
  };

  const fetchModelBackends = async (modelId: string) => {
    setBackendsLoading(true);
    try {
      const res = await api.get(`/api/v1/admin/models/${modelId}/backends`);
      setModelBackends(res.data.data || []);
    } catch {
      messageApi.error('获取后端列表失败');
    } finally {
      setBackendsLoading(false);
    }
  };

  const handleCloseBackendDrawer = () => {
    setBackendDrawerVisible(false);
    setSelectedModelId(null);
    setSelectedModelName('');
    setModelBackends([]);
  };

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

  // Backend management functions
  const handleCreateBackend = () => {
    setEditingBackend(null);
    setBackendModalTitle('创建后端');
    backendForm.resetFields();
    backendForm.setFieldsValue({ weight: 1, enabled: true });
    setBackendModalVisible(true);
  };

  const handleEditBackend = (backend: Backend) => {
    setEditingBackend(backend);
    setBackendModalTitle('编辑后端');
    backendForm.setFieldsValue({
      id: backend.id,
      name: backend.name,
      base_url: backend.base_url,
      model_name: backend.model_name,
      weight: backend.weight,
      region: backend.region,
      enabled: backend.enabled,
    });
    setBackendModalVisible(true);
  };

  const handleDeleteBackend = async (backend: Backend) => {
    if (!selectedModelId) return;
    try {
      await api.delete(`/api/v1/admin/models/${selectedModelId}/backends/${backend.id}`);
      messageApi.success('删除成功');
      fetchModelBackends(selectedModelId);
      fetchData(); // Refresh model list to update backend count
    } catch {
      messageApi.error('删除失败');
    }
  };

  const handleBackendSubmit = async (values: BackendFormValues) => {
    if (!selectedModelId) return;
    try {
      if (editingBackend) {
        await api.put(`/api/v1/admin/models/${selectedModelId}/backends/${values.id}`, values);
        messageApi.success('更新成功');
      } else {
        await api.post(`/api/v1/admin/models/${selectedModelId}/backends`, values);
        messageApi.success('创建成功');
      }
      setBackendModalVisible(false);
      fetchModelBackends(selectedModelId);
      fetchData(); // Refresh model list to update backend count
    } catch (err: unknown) {
      const error = err as { response?: { data?: { error?: string } } };
      messageApi.error(error.response?.data?.error || '操作失败');
    }
  };

  const handleToggleBackendEnabled = async (backend: Backend) => {
    if (!selectedModelId) return;
    try {
      await api.put(`/api/v1/admin/models/${selectedModelId}/backends/${backend.id}`, {
        ...backend,
        region: backend.region,
        enabled: !backend.enabled,
      });
      messageApi.success(backend.enabled ? '后端已禁用' : '后端已启用');
      fetchModelBackends(selectedModelId);
      fetchData(); // Refresh model list to update backend count
    } catch (err: unknown) {
      const error = err as { response?: { data?: { error?: string } } };
      messageApi.error(error.response?.data?.error || '操作失败');
    }
  };

  const userColumns = [
    { title: '邮箱', dataIndex: 'email' },
    { title: '姓名', dataIndex: 'name' },
    { title: '角色', dataIndex: 'role' },
    { title: '部门', dataIndex: 'department' },
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
              onClick={() => handleManageBackends(record.id, record.name)}
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
    { title: '每日请求限额', dataIndex: 'request_quota_daily' },
  ];

  const backendColumns = [
    { title: 'ID', dataIndex: 'id', key: 'id', ellipsis: true },
    {
      title: '名称',
      dataIndex: 'name',
      key: 'name',
      render: (name: string) => name || '-',
    },
    {
      title: 'BaseURL',
      dataIndex: 'base_url',
      key: 'base_url',
      ellipsis: true,
    },
    {
      title: 'ModelName',
      dataIndex: 'model_name',
      key: 'model_name',
      render: (name: string) => name || '-',
    },
    {
      title: '权重',
      dataIndex: 'weight',
      key: 'weight',
      render: (weight: number) => <Tag color="blue">{weight}</Tag>,
    },
    {
      title: 'Region',
      dataIndex: 'region',
      key: 'region',
      render: (region: string) => region || '-',
    },
    {
      title: '健康状态',
      dataIndex: 'healthy',
      key: 'healthy',
      render: (healthy: boolean, record: Backend) => {
        if (!record.enabled) return <Tag>已禁用</Tag>;
        return healthy ? (
          <Tag color="green" icon={<CheckCircleOutlined />}>健康</Tag>
        ) : (
          <Tag color="red" icon={<CloseCircleOutlined />}>不健康</Tag>
        );
      },
    },
    {
      title: '启用状态',
      dataIndex: 'enabled',
      key: 'enabled',
      render: (enabled: boolean, record: Backend) => (
        <Switch
          checked={enabled}
          onChange={() => handleToggleBackendEnabled(record)}
          size="small"
        />
      ),
    },
    {
      title: '最后检查',
      dataIndex: 'last_check_at',
      key: 'last_check_at',
      render: (time: string) => time ? new Date(time).toLocaleString() : '-',
    },
    {
      title: '操作',
      key: 'action',
      render: (_: unknown, record: Backend) => (
        <Space>
          <Tooltip title="编辑">
            <Button
              type="link"
              size="small"
              icon={<EditOutlined />}
              onClick={() => handleEditBackend(record)}
            />
          </Tooltip>
          <Popconfirm
            title="确认删除"
            description={`删除后端 "${record.name || record.id}"，确定要继续吗？`}
            onConfirm={() => handleDeleteBackend(record)}
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

  const activeTabKey = tab || 'users';

  const handleTabChange = (key: string) => {
    navigate(`/admin/${key}`);
  };

  return (
    <div>
      {contextHolder}
      <Card title="管理后台">
        <Tabs activeKey={activeTabKey} onChange={handleTabChange}>
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

      {/* Backend Management Drawer */}
      <Drawer
        title={
          <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
            <Button
              type="text"
              icon={<ArrowLeftOutlined />}
              onClick={handleCloseBackendDrawer}
              style={{ padding: '4px 8px', marginLeft: -8 }}
            />
            <div style={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
              <div style={{ fontSize: 17, fontWeight: 600, lineHeight: 1.4 }}>后端管理</div>
              <div style={{ color: '#8c8c8c', fontSize: 12, lineHeight: 1.4 }}>
                模型: {selectedModelName || selectedModelId}
              </div>
            </div>
          </div>
        }
        placement="right"
        width={1000}
        onClose={handleCloseBackendDrawer}
        open={backendDrawerVisible}
        styles={{ body: { padding: 24, paddingTop: 16 } }}
        closeIcon={null}
      >
        <div style={{ marginBottom: 16, display: 'flex', justifyContent: 'space-between' }}>
          <div>
            <p style={{ color: '#666', margin: 0 }}>
              管理模型 "{selectedModelName || selectedModelId}" 的后端实例，支持负载均衡配置
            </p>
          </div>
          <Space>
            <Button
              icon={<ReloadOutlined />}
              onClick={() => selectedModelId && fetchModelBackends(selectedModelId)}
            >
              刷新
            </Button>
            <Button
              type="primary"
              icon={<PlusOutlined />}
              onClick={handleCreateBackend}
            >
              创建后端
            </Button>
          </Space>
        </div>
        <Table
          dataSource={modelBackends}
          columns={backendColumns}
          rowKey="id"
          loading={backendsLoading}
        />
      </Drawer>

      {/* Backend Create/Edit Modal */}
      <Modal
        title={backendModalTitle}
        open={backendModalVisible}
        onCancel={() => setBackendModalVisible(false)}
        onOk={() => backendForm.submit()}
        okText={editingBackend ? '保存' : '创建'}
        width={600}
        destroyOnClose
      >
        <Form
          form={backendForm}
          onFinish={handleBackendSubmit}
          layout="horizontal"
          labelCol={{ span: 6 }}
          wrapperCol={{ span: 18 }}
          style={{ marginTop: 24 }}
        >
          <Form.Item
            name="id"
            label="后端ID"
            rules={[{ required: true, message: '请输入后端ID' }]}
            extra="唯一标识，如：backend-1, aws-us-east-1"
          >
            <Input disabled={!!editingBackend} placeholder="如：backend-1" />
          </Form.Item>

          <Form.Item
            name="name"
            label="名称"
          >
            <Input placeholder="显示名称（可选）" />
          </Form.Item>

          <Form.Item
            name="base_url"
            label="BaseURL"
            rules={[
              { required: true, message: '请输入BaseURL' },
              { type: 'url', message: '请输入有效的URL' },
            ]}
            extra="后端服务地址，如：http://localhost:8080"
          >
            <Input placeholder="如：http://localhost:8080" />
          </Form.Item>

          <Form.Item
            name="model_name"
            label="ModelName"
            extra="后端实际的模型名称（可选），不填则使用模型ID"
          >
            <Input placeholder="如：gpt-4-turbo" />
          </Form.Item>

          <Form.Item
            name="api_key"
            label="API Key"
            extra={editingBackend ? "留空表示不修改（安全考虑，不显示当前值）" : "后端服务的API密钥（可选）"}
          >
            <Input.Password placeholder="API Key（可选）" />
          </Form.Item>

          <Form.Item
            name="weight"
            label="权重"
            rules={[{ required: true, message: '请输入权重' }]}
            extra="负载均衡权重，数值越大分配越多请求（默认1）"
          >
            <InputNumber min={1} max={100} style={{ width: '100%' }} />
          </Form.Item>

          <Form.Item
            name="region"
            label="Region"
            extra="地区标识（可选），如：cn-north-1, us-west-2"
          >
            <Input placeholder="如：cn-north-1" />
          </Form.Item>

          <Form.Item
            name="enabled"
            label="启用状态"
            valuePropName="checked"
          >
            <Switch />
          </Form.Item>
        </Form>
      </Modal>

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
    </div>
  );
};

export default Admin;
