import React, { useEffect, useState } from 'react';
import { Card, Table, Button, Tag, message, Tabs, Modal, Form, Input, Space, Popconfirm, Statistic, Row, Col, Tooltip, Switch, Drawer, InputNumber, Select } from 'antd';
import { PlusOutlined, EditOutlined, DeleteOutlined, SettingOutlined, ReloadOutlined, CheckCircleOutlined, CloseCircleOutlined, ArrowLeftOutlined, MinusCircleOutlined } from '@ant-design/icons';
import { useParams, useNavigate } from 'react-router-dom';
import api from '../api';
import AdminLogs from './AdminLogs';

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
  enabled: boolean;
  last_login_at?: string;
}

interface TimeRange {
  start: string;
  end: string;
}

interface Policy {
  name: string;
  rate_limit: number;
  rate_limit_window: number;
  request_quota_daily: number;
  available_time_ranges?: TimeRange[];
  models: string[];
  description: string;
  default_model?: string;
}

interface ModelFormValues {
  id: string;
  name: string;
  description: string;
  enabled: boolean;
  model_params?: string;
  base_url?: string;
  api_key?: string;
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

interface UserFormValues {
  id: string;
  email: string;
  password?: string;
  name: string;
  role: string;
  department: string;
  quota_policy: string;
  enabled: boolean;
}

interface PolicyFormValues {
  name: string;
  description: string;
  rate_limit: number;
  rate_limit_window: number;
  request_quota_daily: number;
  available_time_ranges?: TimeRange[];
  models: string[];
  default_model?: string;
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

  // 用户表分页和排序状态
  const [userPagination, setUserPagination] = useState({ current: 1, pageSize: 20, total: 0 });
  const [userSort, setUserSort] = useState({ field: 'created_at', order: 'desc' });

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

  // Import gateway states
  const [importModalVisible, setImportModalVisible] = useState(false);
  const [importLoading, setImportLoading] = useState(false);
  const [importForm] = Form.useForm();

  // User modal states
  const [userModalVisible, setUserModalVisible] = useState(false);
  const [userModalTitle, setUserModalTitle] = useState('创建用户');
  const [editingUser, setEditingUser] = useState<User | null>(null);
  const [userForm] = Form.useForm();

  // Policy modal states
  const [policyModalVisible, setPolicyModalVisible] = useState(false);
  const [policyModalTitle, setPolicyModalTitle] = useState('创建策略');
  const [editingPolicy, setEditingPolicy] = useState<Policy | null>(null);
  const [policyForm] = Form.useForm();

  // System config states
  const [configForm] = Form.useForm();
  const [concurrencyForm] = Form.useForm();

  const [messageApi, contextHolder] = message.useMessage();

  const fetchSystemConfig = async () => {
    try {
      const res = await api.get('/api/v1/config/frontend');
      configForm.setFieldsValue(res.data.data);
    } catch {
      messageApi.error('获取系统配置失败');
    }
  };

  const fetchConcurrencyConfig = async () => {
    try {
      const res = await api.get('/api/v1/admin/config/concurrency');
      concurrencyForm.setFieldsValue(res.data.data);
    } catch {
      messageApi.error('获取并发配置失败');
    }
  };

  const handleSystemConfigSubmit = async (values: any) => {
    try {
      await api.put('/api/v1/admin/config/frontend', values);
      messageApi.success('系统配置保存成功');
      fetchSystemConfig();
    } catch (err: unknown) {
      const error = err as { response?: { data?: { error?: string } } };
      messageApi.error(error.response?.data?.error || '保存系统配置失败');
    }
  };

  const handleConcurrencyConfigSubmit = async (values: { global_limit: number; user_limit: number }) => {
    try {
      await api.put('/api/v1/admin/config/concurrency', values);
      messageApi.success('并发配置保存成功，已动态生效');
      fetchConcurrencyConfig();
    } catch (err: unknown) {
      const error = err as { response?: { data?: { error?: string } } };
      messageApi.error(error.response?.data?.error || '保存并发配置失败');
    }
  };

  // 获取用户列表（支持分页和排序）
  const fetchUsers = async (page = userPagination.current, pageSize = userPagination.pageSize, sortBy = userSort.field, sortOrder = userSort.order) => {
    try {
      const res = await api.get('/api/v1/admin/users', {
        params: { page, page_size: pageSize, sort_by: sortBy, sort_order: sortOrder }
      });
      setUsers(res.data.data || []);
      setUserPagination(prev => ({ ...prev, current: res.data.page, total: res.data.total }));
    } catch (err: unknown) {
      const error = err as { response?: { data?: { error?: string } } };
      messageApi.error(error.response?.data?.error || '获取用户列表失败');
    }
  };

  const fetchData = async () => {
    setLoading(true);
    try {
      const [, modelsRes, policiesRes] = await Promise.all([
        fetchUsers(),
        api.get('/api/v1/admin/models'),
        api.get('/api/v1/admin/policies'),
      ]);
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
    const validTabs = ['users', 'models', 'policies', 'health', 'system', 'logs'];
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
    } else if (currentTab === 'system') {
      fetchSystemConfig();
      fetchConcurrencyConfig();
    }
  }, [tab, models]);

  // Model management functions
  const handleImportGatewaySubmit = async (values: { prefix: string; base_url: string; api_key?: string }) => {
    setImportLoading(true);
    try {
      const res = await api.post('/api/v1/admin/models/import', values);
      messageApi.success(res.data.message || '导入成功');
      setImportModalVisible(false);
      importForm.resetFields();
      fetchData();
    } catch (err: unknown) {
      const error = err as { response?: { data?: { error?: string } } };
      messageApi.error(error.response?.data?.error || '导入失败');
    } finally {
      setImportLoading(false);
    }
  };

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

        // If base_url is provided, also create a backend
        if (values.base_url) {
          try {
            await api.post(`/api/v1/admin/models/${values.id}/backends`, {
              id: `${values.id}-1`,
              base_url: values.base_url,
              api_key: values.api_key || '',
              weight: 1,
              enabled: true,
            });
            messageApi.success('后端创建成功');
          } catch (backendErr: unknown) {
            const backendError = backendErr as { response?: { data?: { error?: string } } };
            messageApi.warning('模型已创建，但后端创建失败：' + (backendError.response?.data?.error || '未知错误'));
          }
        }
      }
      setModelModalVisible(false);
      fetchData();
    } catch (err: unknown) {
      const error = err as { response?: { data?: { error?: string } } };
      messageApi.error(error.response?.data?.error || '操作失败');
    }
  };

  const generateBackendId = () => {
    const existingIds = new Set(modelBackends.map(b => b.id));
    let seq = 1;
    while (existingIds.has(`${selectedModelId}-${seq}`)) {
      seq++;
    }
    return `${selectedModelId}-${seq}`;
  };

  const handleCreateBackend = () => {
    setEditingBackend(null);
    setBackendModalTitle('创建后端');
    backendForm.resetFields();
    backendForm.setFieldsValue({ id: generateBackendId(), weight: 1, enabled: true });
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

  // User management functions
  const handleCreateUser = () => {
    setEditingUser(null);
    setUserModalTitle('创建用户');
    userForm.resetFields();
    userForm.setFieldsValue({ enabled: true, role: 'user', quota_policy: 'default' });
    setUserModalVisible(true);
  };

  const handleEditUser = (user: User) => {
    setEditingUser(user);
    setUserModalTitle('编辑用户');
    userForm.setFieldsValue({
      id: user.id,
      email: user.email,
      name: user.name,
      role: user.role,
      department: user.department,
      quota_policy: user.quota_policy,
      enabled: user.enabled,
    });
    setUserModalVisible(true);
  };

  const handleDeleteUser = async (user: User) => {
    try {
      await api.delete(`/api/v1/admin/users/${user.id}`);
      messageApi.success('用户删除成功');
      fetchData();
    } catch (err: unknown) {
      const error = err as { response?: { data?: { error?: string } } };
      messageApi.error(error.response?.data?.error || '删除失败');
    }
  };

  const handleToggleUserEnabled = async (user: User) => {
    try {
      await api.put(`/api/v1/admin/users/${user.id}`, {
        ...user,
        enabled: !user.enabled,
      });
      messageApi.success(user.enabled ? '用户已禁用' : '用户已启用');
      fetchData();
    } catch (err: unknown) {
      const error = err as { response?: { data?: { error?: string } } };
      messageApi.error(error.response?.data?.error || '操作失败');
    }
  };

  const handleUserSubmit = async (values: UserFormValues) => {
    try {
      if (editingUser) {
        // Update existing user
        await api.put(`/api/v1/admin/users/${editingUser.id}`, {
          name: values.name,
          role: values.role,
          department: values.department,
          quota_policy: values.quota_policy,
          enabled: values.enabled,
        });
        messageApi.success('用户更新成功');
      } else {
        // Create new user
        await api.post('/api/v1/admin/users', {
          id: values.id,
          email: values.email,
          password: values.password,
          name: values.name,
          role: values.role,
          department: values.department,
          quota_policy: values.quota_policy,
          enabled: values.enabled,
        });
        messageApi.success('用户创建成功');
      }
      setUserModalVisible(false);
      fetchData();
    } catch (err: unknown) {
      const error = err as { response?: { data?: { error?: string } } };
      messageApi.error(error.response?.data?.error || '操作失败');
    }
  };

  // Policy management functions
  const handleCreatePolicy = () => {
    setEditingPolicy(null);
    setPolicyModalTitle('创建策略');
    policyForm.resetFields();
    policyForm.setFieldsValue({
      enabled: true,
      rate_limit: 60,
      rate_limit_window: 60,
      request_quota_daily: 1000,
      available_time_ranges: [],
      default_model: '',
    });
    setPolicyModalVisible(true);
  };

  const handleEditPolicy = (policy: Policy) => {
    setEditingPolicy(policy);
    setPolicyModalTitle('编辑策略');
    policyForm.setFieldsValue({
      name: policy.name,
      description: policy.description,
      rate_limit: policy.rate_limit,
      rate_limit_window: policy.rate_limit_window,
      request_quota_daily: policy.request_quota_daily,
      available_time_ranges: policy.available_time_ranges || [],
      models: policy.models || [],
      default_model: policy.default_model,
    });
    setPolicyModalVisible(true);
  };

  const handleDeletePolicy = async (policy: Policy) => {
    try {
      await api.delete(`/api/v1/admin/policies/${policy.name}`);
      messageApi.success('策略删除成功');
      fetchData();
    } catch (err: unknown) {
      const error = err as { response?: { data?: { error?: string } } };
      messageApi.error(error.response?.data?.error || '删除失败');
    }
  };

  const handlePolicySubmit = async (values: PolicyFormValues) => {
    try {
      if (editingPolicy) {
        // Update existing policy
        await api.put(`/api/v1/admin/policies/${editingPolicy.name}`, {
          description: values.description,
          rate_limit: values.rate_limit,
          rate_limit_window: values.rate_limit_window,
          request_quota_daily: values.request_quota_daily,
          available_time_ranges: values.available_time_ranges || [],
          models: values.models,
          default_model: values.default_model,
        });
        messageApi.success('策略更新成功');
      } else {
        // Create new policy
        await api.post('/api/v1/admin/policies', {
          name: values.name,
          description: values.description,
          rate_limit: values.rate_limit,
          rate_limit_window: values.rate_limit_window,
          request_quota_daily: values.request_quota_daily,
          available_time_ranges: values.available_time_ranges || [],
          models: values.models,
          default_model: values.default_model,
        });
        messageApi.success('策略创建成功');
      }
      setPolicyModalVisible(false);
      fetchData();
    } catch (err: unknown) {
      const error = err as { response?: { data?: { error?: string } } };
      messageApi.error(error.response?.data?.error || '操作失败');
    }
  };

  const userColumns = [
    { title: '邮箱', dataIndex: 'email', sorter: true },
    { title: '姓名', dataIndex: 'name' },
    { title: '角色', dataIndex: 'role', render: (role: string) => <Tag color={role === 'admin' ? 'red' : 'default'}>{role}</Tag> },
    { title: '部门', dataIndex: 'department' },
    {
      title: '配额策略',
      dataIndex: 'quota_policy',
      sorter: true,
      render: (policy: string) => policy ? <Tag color="blue">{policy}</Tag> : '-',
    },
    {
      title: '启用状态',
      dataIndex: 'enabled',
      sorter: true,
      render: (enabled: boolean, record: User) => (
        <Space size="small">
          <Switch
            checked={enabled}
            onChange={() => handleToggleUserEnabled(record)}
            size="small"
          />
          {!enabled && !record.last_login_at && (
            <Tag color="orange">待审核</Tag>
          )}
        </Space>
      ),
    },
    {
      title: '最后登录',
      dataIndex: 'last_login_at',
      sorter: true,
      render: (time: string) => time ? new Date(time).toLocaleString() : '-',
    },
    {
      title: '操作',
      render: (_: unknown, record: User) => (
        <Space>
          <Tooltip title="编辑">
            <Button
              type="link"
              size="small"
              icon={<EditOutlined />}
              onClick={() => handleEditUser(record)}
            />
          </Tooltip>
          <Popconfirm
            title="确认删除"
            description={`删除用户 "${record.name || record.email}"，确定要继续吗？`}
            onConfirm={() => handleDeleteUser(record)}
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
    { title: '描述', dataIndex: 'description', ellipsis: true },
    {
      title: '速率限制',
      dataIndex: 'rate_limit',
      render: (v: number, record: Policy) => `${v}/${record.rate_limit_window || 60}s`,
    },
    {
      title: '每日限额',
      dataIndex: 'request_quota_daily',
      render: (v: number) => v === 0 ? <Tag>无限制</Tag> : v,
    },
    {
      title: '关联模型',
      dataIndex: 'models',
      render: (models: string[]) =>
        models?.length > 0
          ? <Space size="small">{models.map(m => <Tag key={m}>{m}</Tag>)}</Space>
          : '-',
    },
    {
      title: '可用时段',
      dataIndex: 'available_time_ranges',
      render: (ranges: TimeRange[]) =>
        ranges && ranges.length > 0
          ? <Space size="small" wrap>{ranges.map((r, i) => <Tag key={i} color="cyan">{r.start}-{r.end}</Tag>)}</Space>
          : <Tag>全天</Tag>,
    },
    {
      title: '默认模型',
      dataIndex: 'default_model',
      render: (model: string) => model ? <Tag color="blue">{model}</Tag> : '-',
    },
    {
      title: '操作',
      render: (_: unknown, record: Policy) => (
        <Space>
          <Tooltip title="编辑">
            <Button
              type="link"
              size="small"
              icon={<EditOutlined />}
              onClick={() => handleEditPolicy(record)}
            />
          </Tooltip>
          <Popconfirm
            title="确认删除"
            description={`删除策略 "${record.name}"，确定要继续吗？`}
            onConfirm={() => handleDeletePolicy(record)}
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
      render: (time: any) => {
        if (!time) return '-';
        const dateStr = typeof time === 'string' ? time : (time.Time || '');
        if (!dateStr || dateStr.startsWith('0001-01-01')) return '-';
        const d = new Date(dateStr);
        return isNaN(d.getTime()) ? '-' : d.toLocaleString();
      },
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
        // 如果 Backend 被禁用，显示已禁用状态
        if (record.enabled === false) {
          return <Tag color="default">已禁用</Tag>;
        }

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
        const lastCheckVal = health.last_check as any;
        const dateStr = typeof lastCheckVal === 'string' ? lastCheckVal : (lastCheckVal.Time || '');
        if (!dateStr || dateStr.startsWith('0001-01-01')) return '-';
        const d = new Date(dateStr);
        return isNaN(d.getTime()) ? '-' : d.toLocaleString();
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
            <div style={{ marginBottom: 16 }}>
              <Button
                type="primary"
                icon={<PlusOutlined />}
                onClick={handleCreateUser}
              >
                创建用户
              </Button>
            </div>
            <Table
              dataSource={users}
              columns={userColumns}
              rowKey="id"
              loading={loading}
              pagination={{
                current: userPagination.current,
                pageSize: userPagination.pageSize,
                total: userPagination.total,
                showSizeChanger: true,
                pageSizeOptions: ['20', '30', '50'],
                showTotal: (total) => `共 ${total} 个用户`,
              }}
              onChange={(pagination, _filters, sorter) => {
                const s = Array.isArray(sorter) ? sorter[0] : sorter;
                const newPage = pagination.current || 1;
                const newPageSize = pagination.pageSize || 20;
                const newSortField = (s?.field as string) || 'created_at';
                const newSortOrder = s?.order === 'ascend' ? 'asc' : 'desc';
                setUserPagination(prev => ({ ...prev, current: newPage, pageSize: newPageSize }));
                setUserSort({ field: newSortField, order: newSortOrder });
                fetchUsers(newPage, newPageSize, newSortField, newSortOrder);
              }}
            />
          </Tabs.TabPane>
          <Tabs.TabPane tab="模型管理" key="models">
            <div style={{ marginBottom: 16 }}>
              <Space>
                <Button
                  type="primary"
                  icon={<PlusOutlined />}
                  onClick={handleCreateModel}
                >
                  创建模型
                </Button>
                <Button
                  icon={<PlusOutlined />}
                  onClick={() => {
                    importForm.resetFields();
                    setImportModalVisible(true);
                  }}
                >
                  从网关导入
                </Button>
              </Space>
            </div>
            <Table
              dataSource={models}
              columns={modelColumns}
              rowKey="id"
              loading={loading}
            />
          </Tabs.TabPane>
          <Tabs.TabPane tab="配额策略" key="policies">
            <div style={{ marginBottom: 16 }}>
              <Button
                type="primary"
                icon={<PlusOutlined />}
                onClick={handleCreatePolicy}
              >
                创建策略
              </Button>
            </div>
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
          <Tabs.TabPane tab="系统配置" key="system">
            <Row gutter={16}>
              <Col span={12}>
                <Card title="前端配置" style={{ marginBottom: 16 }}>
                  <Form
                    form={configForm}
                    layout="horizontal"
                    labelCol={{ span: 6 }}
                    wrapperCol={{ span: 18 }}
                    onFinish={handleSystemConfigSubmit}
                  >
                    <Form.Item
                      name="feedback_url"
                      label="反馈链接"
                      rules={[{ type: 'url', message: '请输入有效的URL' }]}
                    >
                      <Input placeholder="如：https://example.com/feedback" />
                    </Form.Item>
                    <Form.Item
                      name="dev_manual_url"
                      label="开发手册链接"
                      rules={[{ type: 'url', message: '请输入有效的URL' }]}
                    >
                      <Input placeholder="如：https://example.com/docs" />
                    </Form.Item>
                    <Form.Item
                      name="sso_enabled"
                      label="SSO 启用"
                      valuePropName="checked"
                    >
                      <Switch disabled />
                    </Form.Item>
                    <Form.Item wrapperCol={{ offset: 6, span: 18 }}>
                      <Space>
                        <Button type="primary" onClick={() => configForm.submit()}>
                          保存
                        </Button>
                        <Button
                          icon={<ReloadOutlined />}
                          onClick={fetchSystemConfig}
                        >
                          刷新
                        </Button>
                      </Space>
                    </Form.Item>
                  </Form>
                </Card>
                <Card title="并发控制" style={{ marginBottom: 16 }}>
                  <Form
                    form={concurrencyForm}
                    layout="horizontal"
                    labelCol={{ span: 8 }}
                    wrapperCol={{ span: 16 }}
                    onFinish={handleConcurrencyConfigSubmit}
                  >
                    <Form.Item
                      name="global_limit"
                      label="全局并发限制"
                      rules={[{ required: true, message: '请输入全局并发限制' }]}
                      extra="全局最大并发请求数，0 表示不限制"
                    >
                      <InputNumber min={0} max={10000} style={{ width: '100%' }} placeholder="如：100" />
                    </Form.Item>
                    <Form.Item
                      name="user_limit"
                      label="用户并发限制"
                      rules={[{ required: true, message: '请输入用户并发限制' }]}
                      extra="每个用户最大并发请求数，0 表示不限制"
                    >
                      <InputNumber min={0} max={1000} style={{ width: '100%' }} placeholder="如：10" />
                    </Form.Item>
                    <Form.Item wrapperCol={{ offset: 8, span: 16 }}>
                      <Space>
                        <Button type="primary" onClick={() => concurrencyForm.submit()}>
                          保存
                        </Button>
                        <Button
                          icon={<ReloadOutlined />}
                          onClick={fetchConcurrencyConfig}
                        >
                          刷新
                        </Button>
                      </Space>
                    </Form.Item>
                  </Form>
                </Card>
              </Col>
              <Col span={12}>
                <Card title="系统统计">
                  <Row gutter={[16, 16]}>
                    <Col span={12}>
                      <Statistic
                        title="总用户数"
                        value={users.length}
                        valueStyle={{ color: '#1890ff' }}
                      />
                    </Col>
                    <Col span={12}>
                      <Statistic
                        title="总模型数"
                        value={models.length}
                        valueStyle={{ color: '#52c41a' }}
                      />
                    </Col>
                    <Col span={12}>
                      <Statistic
                        title="总策略数"
                        value={policies.length}
                        valueStyle={{ color: '#722ed1' }}
                      />
                    </Col>
                    <Col span={12}>
                      <Statistic
                        title="总后端数"
                        value={backends.length}
                        valueStyle={{ color: '#fa8c16' }}
                      />
                    </Col>
                  </Row>
                </Card>
              </Col>
            </Row>
          </Tabs.TabPane>
          <Tabs.TabPane tab="全员访问日志" key="logs">
            <AdminLogs />
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

          {!editingModel && (
            <>
              <Form.Item style={{ marginBottom: 8 }}>
                <div style={{ borderTop: '1px solid #f0f0f0', paddingTop: 16, color: '#8c8c8c', fontSize: 13 }}>
                  以下为可选项，填写后将同时创建一个后端实例
                </div>
              </Form.Item>
              <Form.Item
                name="base_url"
                label="BaseURL"
                rules={[
                  { type: 'url', message: '请输入有效的URL' },
                ]}
                extra="后端服务地址，如：https://api.openai.com"
              >
                <Input placeholder="如：https://api.openai.com" />
              </Form.Item>
              <Form.Item
                name="api_key"
                label="API Key"
                extra="后端服务的API密钥（可选）"
              >
                <Input.Password placeholder="API Key（可选）" />
              </Form.Item>
            </>
          )}
        </Form>
      </Modal>

      {/* User Create/Edit Modal */}
      <Modal
        title={userModalTitle}
        open={userModalVisible}
        onCancel={() => setUserModalVisible(false)}
        onOk={() => userForm.submit()}
        okText={editingUser ? '保存' : '创建'}
        width={600}
        destroyOnClose
      >
        <Form
          form={userForm}
          onFinish={handleUserSubmit}
          layout="horizontal"
          labelCol={{ span: 6 }}
          wrapperCol={{ span: 18 }}
          style={{ marginTop: 24 }}
        >
          <Form.Item
            name="id"
            label="用户ID"
            rules={[{ required: !editingUser, message: '请输入用户ID' }]}
            extra="唯一标识，如：zhangsan, lisi"
            hidden={!!editingUser}
          >
            <Input disabled={!!editingUser} placeholder="如：zhangsan" />
          </Form.Item>

          <Form.Item
            name="email"
            label="邮箱"
            rules={[
              { required: true, message: '请输入邮箱' },
              { type: 'email', message: '请输入有效的邮箱地址' },
            ]}
          >
            <Input disabled={!!editingUser} placeholder="如：user@example.com" />
          </Form.Item>

          {!editingUser && (
            <Form.Item
              name="password"
              label="密码"
              rules={[{ required: !editingUser, message: '请输入密码' }]}
              extra="初始密码，创建后建议用户修改"
            >
              <Input.Password placeholder="请输入初始密码" />
            </Form.Item>
          )}

          <Form.Item
            name="name"
            label="姓名"
            rules={[{ required: true, message: '请输入姓名' }]}
          >
            <Input placeholder="如：张三" />
          </Form.Item>

          <Form.Item
            name="role"
            label="角色"
            rules={[{ required: true, message: '请选择角色' }]}
          >
            <Select placeholder="请选择角色">
              <Select.Option value="admin">管理员</Select.Option>
              <Select.Option value="manager">经理</Select.Option>
              <Select.Option value="user">普通用户</Select.Option>
            </Select>
          </Form.Item>

          <Form.Item
            name="department"
            label="部门"
          >
            <Input placeholder="如：技术部（可选）" />
          </Form.Item>

          <Form.Item
            name="quota_policy"
            label="配额策略"
            rules={[{ required: true, message: '请选择配额策略' }]}
          >
            <Select placeholder="请选择配额策略">
              {policies.map(policy => (
                <Select.Option key={policy.name} value={policy.name}>{policy.name}</Select.Option>
              ))}
            </Select>
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

      {/* Policy Create/Edit Modal */}
      <Modal
        title={policyModalTitle}
        open={policyModalVisible}
        onCancel={() => setPolicyModalVisible(false)}
        onOk={() => policyForm.submit()}
        okText={editingPolicy ? '保存' : '创建'}
        width={600}
        destroyOnClose
      >
        <Form
          form={policyForm}
          onFinish={handlePolicySubmit}
          layout="horizontal"
          labelCol={{ span: 6 }}
          wrapperCol={{ span: 18 }}
          style={{ marginTop: 24 }}
        >
          <Form.Item
            name="name"
            label="策略名称"
            rules={[{ required: !editingPolicy, message: '请输入策略名称' }]}
            extra="唯一标识，如：default, premium"
            hidden={!!editingPolicy}
          >
            <Input disabled={!!editingPolicy} placeholder="如：premium" />
          </Form.Item>

          <Form.Item
            name="description"
            label="描述"
          >
            <Input.TextArea rows={2} placeholder="策略描述信息（可选）" />
          </Form.Item>

          <Form.Item
            name="rate_limit"
            label="速率限制"
            rules={[{ required: true, message: '请输入速率限制' }]}
            extra="单位时间内允许的请求次数"
          >
            <InputNumber min={1} max={10000} style={{ width: '100%' }} placeholder="如：60" />
          </Form.Item>

          <Form.Item
            name="rate_limit_window"
            label="时间窗口"
            rules={[{ required: true, message: '请输入时间窗口' }]}
            extra="速率限制的时间窗口（秒）"
          >
            <InputNumber min={1} max={3600} style={{ width: '100%' }} placeholder="如：60" />
          </Form.Item>

          <Form.Item
            name="request_quota_daily"
            label="每日限额"
            rules={[{ required: true, message: '请输入每日限额' }]}
            extra="每天允许的请求次数（0表示无限制）"
          >
            <InputNumber min={0} max={1000000} style={{ width: '100%' }} placeholder="如：1000" />
          </Form.Item>

          <Form.Item label="可用时段" extra="不添加任何时段表示全天可用，支持跨午夜（如 22:00-06:00）">
            <Form.List name="available_time_ranges">
              {(fields, { add, remove }) => (
                <>
                  {fields.map(({ key, name, ...restField }) => (
                    <Space key={key} style={{ display: 'flex', marginBottom: 8 }} align="baseline">
                      <Form.Item
                        {...restField}
                        name={[name, 'start']}
                        rules={[{ required: true, message: '请输入开始时间' }, { pattern: /^([01]\d|2[0-4]):([0-5]\d)$/, message: 'HH:MM 格式' }]}
                        style={{ marginBottom: 0 }}
                      >
                        <Input placeholder="00:00" style={{ width: 90 }} />
                      </Form.Item>
                      <span>-</span>
                      <Form.Item
                        {...restField}
                        name={[name, 'end']}
                        rules={[{ required: true, message: '请输入结束时间' }, { pattern: /^([01]\d|2[0-4]):([0-5]\d)$/, message: 'HH:MM 格式' }]}
                        style={{ marginBottom: 0 }}
                      >
                        <Input placeholder="24:00" style={{ width: 90 }} />
                      </Form.Item>
                      <MinusCircleOutlined onClick={() => remove(name)} style={{ color: '#ff4d4f' }} />
                    </Space>
                  ))}
                  <Button type="dashed" onClick={() => add()} block icon={<PlusOutlined />}>
                    添加时段
                  </Button>
                </>
              )}
            </Form.List>
          </Form.Item>

          <Form.Item
            name="models"
            label="关联模型"
            extra="选择该策略允许的模型（多选）"
          >
            <Select
              mode="multiple"
              placeholder="请选择关联模型"
              style={{ width: '100%' }}
              options={models.map(model => ({ label: model.name, value: model.id }))}
            />
          </Form.Item>
          <Form.Item
            name="default_model"
            label="默认模型"
            extra="该策略所对应的默认回退模型（可选）"
          >
            <Select
              allowClear
              placeholder="请选择默认模型"
              style={{ width: '100%' }}
              options={models.map(model => ({ label: model.name, value: model.id }))}
            />
          </Form.Item>
        </Form>
      </Modal>

      {/* Import Gateway Modal */}
      <Modal
        title="从网关批量导入模型"
        open={importModalVisible}
        onCancel={() => setImportModalVisible(false)}
        onOk={() => importForm.submit()}
        confirmLoading={importLoading}
        okText="导入"
        width={600}
      >
        <Form
          form={importForm}
          onFinish={handleImportGatewaySubmit}
          layout="vertical"
          style={{ marginTop: 24 }}
        >
          <Form.Item
            name="prefix"
            label="前缀 (Prefix)"
            rules={[
              { required: true, message: '请输入前缀' },
              { pattern: /^[a-zA-Z0-9]+$/, message: '前缀只能包含英文字母和数字' }
            ]}
            extra="用于生成后端ID（格式：前缀-模型名-序号），例如：google"
          >
            <Input placeholder="输入服务提供商前缀，如 google, azure, openai" />
          </Form.Item>

          <Form.Item
            name="base_url"
            label="BaseURL"
            rules={[
              { required: true, message: '请输入上游网关 BaseURL' },
              { type: 'url', message: '请输入有效的 URL' }
            ]}
            extra="上游网关服务地址，系统会调用 {BaseURL}/v1/models"
          >
            <Input placeholder="例如 https://api.openai.com" />
          </Form.Item>

          <Form.Item
            name="api_key"
            label="API Key"
            extra="访问上游网关所需的 API Key（可选）"
          >
            <Input.Password placeholder="API Key" />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
};

export default Admin;
