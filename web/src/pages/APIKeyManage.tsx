import React, { useEffect, useState } from 'react';
import { Card, Button, Table, Tag, message, Modal, Form, Input, Space, Popconfirm } from 'antd';
import { PlusOutlined, CopyOutlined, DeleteOutlined, EyeOutlined, EyeInvisibleOutlined } from '@ant-design/icons';
import api from '../api';

interface APIKey {
  id: string;
  name: string;
  key?: string;
  key_prefix: string;
  key_hash?: string;
  created_at: string;
  last_used_at?: string;
  enabled: boolean;
  expires_at?: string;
  total_tokens_used?: number;
}

const APIKeyManage: React.FC = () => {
  const [keys, setKeys] = useState<APIKey[]>([]);
  const [loading, setLoading] = useState(false);
  const [modalVisible, setModalVisible] = useState(false);
  const [newKey, setNewKey] = useState('');
  const [form] = Form.useForm();
  const [showKey, setShowKey] = useState<Record<string, boolean>>({});

  useEffect(() => {
    fetchKeys();
  }, []);

  const fetchKeys = async () => {
    setLoading(true);
    try {
      const res = await api.get('/api/v1/user/keys');
      setKeys(res.data.data || []);
    } catch (err) {
      message.error('获取 API Keys 失败');
    } finally {
      setLoading(false);
    }
  };

  const handleCreate = async (values: { name: string }) => {
    try {
      const res = await api.post('/api/v1/user/keys', values);
      setNewKey(res.data.data?.key || '');
      form.resetFields();
      fetchKeys();
    } catch (err) {
      message.error('创建失败');
    }
  };

  const handleDelete = async (id: string) => {
    try {
      await api.delete(`/api/v1/user/keys/${id}`);
      message.success('删除成功');
      fetchKeys();
    } catch (err) {
      message.error('删除失败');
    }
  };

  const copyToClipboard = async (text: string) => {
    try {
      if (navigator.clipboard && window.isSecureContext) {
        await navigator.clipboard.writeText(text);
        message.success('已复制到剪贴板');
      } else {
        // Fallback for non-secure contexts (HTTP)
        const textArea = document.createElement("textarea");
        textArea.value = text;
        textArea.style.position = "absolute";
        textArea.style.opacity = "0";
        textArea.style.left = "-999999px";
        textArea.style.top = "-999999px";
        document.body.appendChild(textArea);
        textArea.focus();
        textArea.select();
        try {
          const successful = document.execCommand('copy');
          if (successful) {
            message.success('已复制到剪贴板');
          } else {
            message.error('复制失败，请手动复制');
          }
        } catch (err) {
          console.error('Fallback copy error', err);
          message.error('复制失败，请手动复制');
        } finally {
          textArea.remove();
        }
      }
    } catch (err) {
      console.error('Copy error', err);
      message.error('复制失败，请手动复制');
    }
  };

  const toggleShowKey = (id: string) => {
    setShowKey(prev => ({ ...prev, [id]: !prev[id] }));
  };

  const columns = [
    {
      title: '名称',
      dataIndex: 'name',
      key: 'name',
    },
    {
      title: 'API Key',
      key: 'key',
      render: (_: any, record: APIKey) => {
        const isShow = showKey[record.id];
        return (
          <Space>
            <span style={{ fontFamily: 'monospace' }}>
              {isShow ? (record.key || `${record.key_prefix}****************`) : `${record.key_prefix}****`}
            </span>
            <Button
              type="link"
              size="small"
              icon={isShow ? <EyeInvisibleOutlined /> : <EyeOutlined />}
              onClick={() => toggleShowKey(record.id)}
            />
          </Space>
        );
      },
    },
    {
      title: '状态',
      dataIndex: 'enabled',
      key: 'enabled',
      render: (enabled: boolean) => (
        <Tag color={enabled ? 'green' : 'red'}>
          {enabled ? '启用' : '禁用'}
        </Tag>
      ),
    },
    {
      title: '创建时间',
      dataIndex: 'created_at',
      key: 'created_at',
      render: (text: string) => new Date(text).toLocaleString(),
    },
    {
      title: 'Token消耗',
      dataIndex: 'total_tokens_used',
      key: 'total_tokens_used',
      render: (tokens: number | undefined) => tokens ? tokens.toLocaleString() : '0',
    },
    {
      title: '最后使用',
      dataIndex: 'last_used_at',
      key: 'last_used_at',
      render: (text: string) => text ? new Date(text).toLocaleString() : '从未使用',
    },
    {
      title: '操作',
      key: 'action',
      render: (_: any, record: APIKey) => (
        <Popconfirm
          title="确认删除"
          description="删除后该 API Key 将无法使用，确定要继续吗？"
          onConfirm={() => handleDelete(record.id)}
          okText="删除"
          cancelText="取消"
          okButtonProps={{ danger: true }}
        >
          <Button
            type="link"
            danger
            icon={<DeleteOutlined />}
          >
            删除
          </Button>
        </Popconfirm>
      ),
    },
  ];

  return (
    <div>
      <div style={{ marginBottom: 24, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <div>
          <h2 style={{ margin: 0 }}>API Key 管理</h2>
          <p style={{ color: '#666', margin: '8px 0 0 0' }}>
            管理您的 API Keys，用于调用 LLM 接口。请妥善保管，不要泄露给他人。
          </p>
        </div>
        <Button
          type="primary"
          icon={<PlusOutlined />}
          onClick={() => setModalVisible(true)}
        >
          创建 API Key
        </Button>
      </div>

      <Card>
        <Table
          dataSource={keys}
          columns={columns}
          rowKey="id"
          loading={loading}
          pagination={false}
        />
      </Card>

      {/* 创建 API Key 弹窗 */}
      <Modal
        title="创建 API Key"
        open={modalVisible}
        onCancel={() => {
          setModalVisible(false);
          setNewKey('');
        }}
        footer={null}
        width={600}
      >
        {newKey ? (
          <div style={{ padding: '20px 0' }}>
            <div style={{
              background: '#f6ffed',
              border: '1px solid #b7eb8f',
              borderRadius: 4,
              padding: 16,
              marginBottom: 16,
            }}>
              <p style={{ margin: '0 0 12px 0', fontWeight: 'bold', color: '#52c41a' }}>
                API Key 创建成功！
              </p>
              <p style={{ margin: '0 0 8px 0', color: '#666' }}>
                请妥善保管您的 API Key：
              </p>
              <div style={{
                background: '#fff',
                border: '1px dashed #d9d9d9',
                borderRadius: 4,
                padding: 12,
                fontFamily: 'monospace',
                fontSize: 14,
                wordBreak: 'break-all',
              }}>
                {newKey}
              </div>
            </div>
            <div style={{ display: 'flex', gap: 12, justifyContent: 'flex-end' }}>
              <Button
                icon={<CopyOutlined />}
                onClick={() => copyToClipboard(newKey)}
              >
                复制
              </Button>
              <Button
                type="primary"
                onClick={() => {
                  setModalVisible(false);
                  setNewKey('');
                }}
              >
                完成
              </Button>
            </div>
          </div>
        ) : (
          <Form
            form={form}
            onFinish={handleCreate}
            layout="vertical"
          >
            <Form.Item
              name="name"
              label="Key 名称"
              rules={[{ required: true, message: '请输入 Key 名称' }]}
              help="用于标识这个 Key 的用途，如：开发测试、生产环境"
            >
              <Input placeholder="如：开发测试" />
            </Form.Item>

            <Form.Item style={{ marginBottom: 0, textAlign: 'right' }}>
              <Button style={{ marginRight: 8 }} onClick={() => setModalVisible(false)}>
                取消
              </Button>
              <Button type="primary" htmlType="submit">
                创建
              </Button>
            </Form.Item>
          </Form>
        )}
      </Modal>
    </div>
  );
};

export default APIKeyManage;
