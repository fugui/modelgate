import React, { useEffect, useState } from 'react';
import { Layout, Card, Button, Table, Tag, message, Modal, Form, Input, Descriptions } from 'antd';
import { PlusOutlined, CopyOutlined, DeleteOutlined, LogoutOutlined } from '@ant-design/icons';
import { useNavigate } from 'react-router-dom';
import api from '../api';

const { Header, Content } = Layout;

interface APIKey {
  id: string;
  name: string;
  key_prefix: string;
  created_at: string;
  last_used_at?: string;
  enabled: boolean;
  total_tokens_used?: number;
}

const Dashboard: React.FC = () => {
  const [keys, setKeys] = useState<APIKey[]>([]);
  const [quota, setQuota] = useState<any>({});
  const [modalVisible, setModalVisible] = useState(false);
  const [newKey, setNewKey] = useState('');
  const [form] = Form.useForm();
  const navigate = useNavigate();
  
  const storedUser = localStorage.getItem('user');
  const user = storedUser && storedUser !== 'undefined' ? JSON.parse(storedUser) : {};

  const [messageApi, contextHolder] = message.useMessage();

  const fetchData = async () => {
    try {
      const [keysRes, quotaRes] = await Promise.all([
        api.get('/api/v1/user/keys'),
        api.get('/api/v1/user/quota'),
      ]);
      setKeys(keysRes.data.data || []);
      setQuota(quotaRes.data.data || {});
    } catch (err) {
      messageApi.error('获取数据失败');
    }
  };

  useEffect(() => {
    fetchData();
  }, []);

  const handleCreate = async (values: { name: string }) => {
    try {
      const res = await api.post('/api/v1/user/keys', values);
      setNewKey(res.data.key);
      setModalVisible(false);
      form.resetFields();
      fetchData();
    } catch (err) {
      messageApi.error('创建失败');
    }
  };

  const handleDelete = async (id: string) => {
    Modal.confirm({
      title: '确认删除',
      content: '删除后该 API Key 将无法使用',
      onOk: async () => {
        try {
          await api.delete(`/api/v1/user/keys/${id}`);
              messageApi.success('删除成功');
          fetchData();
        } catch (err) {
              messageApi.error('删除失败');
        }
      },
    });
  };

  const logout = () => {
    localStorage.clear();
    navigate('/login');
  };

  const columns = [
    { title: '名称', dataIndex: 'name' },
    { title: 'Key', render: (r: APIKey) => `${r.key_prefix}****` },
    { title: 'Token消耗', dataIndex: 'total_tokens_used', render: (v: number) => v ? v.toLocaleString() : '0' },
    { title: '创建时间', dataIndex: 'created_at', render: (text: string) => new Date(text).toLocaleString() },
    { 
      title: '状态', 
      dataIndex: 'enabled',
      render: (v: boolean) => v ? <Tag color="green">启用</Tag> : <Tag>禁用</Tag>
    },
    {
      title: '操作',
      render: (_: any, record: APIKey) => (
        <Button 
          icon={<DeleteOutlined />} 
          danger 
          size="small"
          onClick={() => handleDelete(record.id)}
        >
          删除
        </Button>
      ),
    },
  ];

  return (
    <Layout style={{ minHeight: '100vh' }}>
      <Header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <h2 style={{ color: '#fff', margin: 0 }}>LLMGATE 控制台</h2>
        <div>
          <span style={{ color: '#fff', marginRight: 16 }}>{user.name}</span>
          {user.role === 'admin' && (
            <Button type="link" onClick={() => navigate('/admin')}>管理后台</Button>
          )}
          <Button icon={<LogoutOutlined />} onClick={logout}>退出</Button>
        </div>
      </Header>
      <Content style={{ padding: 24 }}>
        {contextHolder}
        <Card title="配额使用情况" style={{ marginBottom: 24 }}>
          <Descriptions>
            <Descriptions.Item label="速率限制">{quota.rate_limit} 请求/分钟</Descriptions.Item>
            <Descriptions.Item label="请求配额">{quota.daily_requests_used} / {quota.daily_requests_limit}</Descriptions.Item>
            <Descriptions.Item label="可用模型">{quota.models?.join(', ')}</Descriptions.Item>
          </Descriptions>
        </Card>

        <Card 
          title="API Keys" 
          extra={
            <Button type="primary" icon={<PlusOutlined />} onClick={() => setModalVisible(true)}>
              创建 Key
            </Button>
          }
        >
          <Table dataSource={keys} columns={columns} rowKey="id" />
        </Card>

        <Modal
          title="创建 API Key"
          open={modalVisible}
          onCancel={() => setModalVisible(false)}
          footer={null}
        >
          {newKey ? (
            <div>
              <p>请保存您的 API Key，它只会显示一次：</p>
              <pre style={{ background: '#f5f5f5', padding: 16 }}>{newKey}</pre>
              <Button 
                icon={<CopyOutlined />} 
                onClick={() => {
                  navigator.clipboard.writeText(newKey);
                  messageApi.success('已复制');
                }}
              >
                复制
              </Button>
              <Button onClick={() => { setNewKey(''); setModalVisible(false); }} style={{ marginLeft: 8 }}>
                关闭
              </Button>
            </div>
          ) : (
            <Form form={form} onFinish={handleCreate}>
              <Form.Item name="name" rules={[{ required: true }]} label="名称">
                <Input placeholder="如：开发测试" />
              </Form.Item>
              <Button type="primary" htmlType="submit">创建</Button>
            </Form>
          )}
        </Modal>
      </Content>
    </Layout>
  );
};

export default Dashboard;