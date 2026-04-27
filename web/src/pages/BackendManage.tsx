import React, { useEffect, useState } from 'react';
import { Card, Table, Button, Tag, message, Modal, Form, Input, InputNumber, Space, Popconfirm, Tooltip, Switch } from 'antd';
import { ArrowLeftOutlined, PlusOutlined, EditOutlined, DeleteOutlined, ReloadOutlined, CheckCircleOutlined, CloseCircleOutlined } from '@ant-design/icons';
import { useNavigate, useParams } from 'react-router-dom';
import api from '../api';

interface Backend {
  id: string;
  model_id: string;
  base_url: string;
  model_name: string;
  weight: number;
  enabled: boolean;
  healthy: boolean;
  last_check_at: string;
  created_at: string;
  updated_at: string;
}

interface Model {
  id: string;
  name: string;
  description: string;
  enabled: boolean;
}

interface BackendFormValues {
  id: string;
  base_url: string;
  model_name: string;
  api_key: string;
  weight: number;
  enabled: boolean;
}

const BackendManage: React.FC = () => {
  const { modelId } = useParams<{ modelId: string }>();
  const navigate = useNavigate();
  const [backends, setBackends] = useState<Backend[]>([]);
  const [model, setModel] = useState<Model | null>(null);
  const [loading, setLoading] = useState(false);

  // Modal states
  const [backendModalVisible, setBackendModalVisible] = useState(false);
  const [backendModalTitle, setBackendModalTitle] = useState('创建后端');
  const [editingBackend, setEditingBackend] = useState<Backend | null>(null);
  const [backendForm] = Form.useForm();

  const [messageApi, contextHolder] = message.useMessage();

  const fetchModel = async () => {
    if (!modelId) return;
    try {
      const res = await api.get('/api/v1/admin/models');
      const models = res.data.data || [];
      const found = models.find((m: Model) => m.id === modelId);
      if (found) {
        setModel(found);
      } else {
        messageApi.error('模型不存在');
        navigate('/admin');
      }
    } catch {
      messageApi.error('获取模型信息失败');
    }
  };

  const fetchBackends = async () => {
    if (!modelId) return;
    setLoading(true);
    try {
      const res = await api.get(`/api/v1/admin/models/${modelId}/backends`);
      setBackends(res.data.data || []);
    } catch {
      messageApi.error('获取后端列表失败');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchModel();
    fetchBackends();
  }, [modelId]);

  const generateBackendId = () => {
    const existingIds = new Set(backends.map(b => b.id));
    let seq = 1;
    while (existingIds.has(`${modelId}-${seq}`)) {
      seq++;
    }
    return `${modelId}-${seq}`;
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
      base_url: backend.base_url,
      model_name: backend.model_name,
      weight: backend.weight,
      enabled: backend.enabled,
    });
    setBackendModalVisible(true);
  };

  const handleDeleteBackend = async (backend: Backend) => {
    try {
      await api.delete(`/api/v1/admin/models/${modelId}/backends/${backend.id}`);
      messageApi.success('删除成功');
      fetchBackends();
    } catch {
      messageApi.error('删除失败');
    }
  };

  const handleBackendSubmit = async (values: BackendFormValues) => {
    try {
      if (editingBackend) {
        await api.put(`/api/v1/admin/models/${modelId}/backends/${values.id}`, values);
        messageApi.success('更新成功');
      } else {
        await api.post(`/api/v1/admin/models/${modelId}/backends`, values);
        messageApi.success('创建成功');
      }
      setBackendModalVisible(false);
      fetchBackends();
    } catch (err: unknown) {
      const error = err as { response?: { data?: { error?: string } } };
      messageApi.error(error.response?.data?.error || '操作失败');
    }
  };

  const handleToggleBackendEnabled = async (backend: Backend) => {
    try {
      await api.put(`/api/v1/admin/models/${modelId}/backends/${backend.id}`, {
        ...backend,
        enabled: !backend.enabled,
      });
      messageApi.success(backend.enabled ? '后端已禁用' : '后端已启用');
      fetchBackends();
    } catch (err: unknown) {
      const error = err as { response?: { data?: { error?: string } } };
      messageApi.error(error.response?.data?.error || '操作失败');
    }
  };

  const backendColumns = [
    { title: 'ID', dataIndex: 'id', key: 'id', ellipsis: true },
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
            description={`删除后端 "${record.id}"，确定要继续吗？`}
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

  return (
    <div>
      {contextHolder}
      <Card
        title={
          <div style={{ display: 'flex', alignItems: 'center', gap: 16 }}>
            <Button
              icon={<ArrowLeftOutlined />}
              onClick={() => navigate('/admin')}
            >
              返回
            </Button>
            <div>
              <div style={{ fontSize: 16, fontWeight: 500 }}>后端管理</div>
              <div style={{ color: '#666', fontSize: 12 }}>
                模型: {model?.name || modelId}
              </div>
            </div>
          </div>
        }
      >
        <div style={{ marginBottom: 16, display: 'flex', justifyContent: 'space-between' }}>
          <div>
            <p style={{ color: '#666', margin: 0 }}>
              管理模型 &quot;{model?.name || modelId}&quot; 的后端实例，支持负载均衡配置
            </p>
          </div>
          <Space>
            <Button
              icon={<ReloadOutlined />}
              onClick={fetchBackends}
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
          dataSource={backends}
          columns={backendColumns}
          rowKey="id"
          loading={loading}
        />
      </Card>

      {/* Backend Create/Edit Modal */}
      <Modal
        title={backendModalTitle}
        open={backendModalVisible}
        onCancel={() => setBackendModalVisible(false)}
        onOk={() => backendForm.submit()}
        okText={editingBackend ? '保存' : '创建'}
        width={600}
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
            name="enabled"
            label="启用状态"
            valuePropName="checked"
          >
            <Switch />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
};

export default BackendManage;
