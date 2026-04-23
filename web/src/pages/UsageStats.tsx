import React, { useEffect, useState } from 'react';
import { Card, Descriptions, Table, Tag, Statistic, Row, Col, Progress, Modal, Button, Tooltip as AntTooltip } from 'antd';
import { BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer } from 'recharts';
import { EyeOutlined } from '@ant-design/icons';
import api from '../api';

const UsageStats: React.FC = () => {
  const [quota, setQuota] = useState<any>({});
  const [_usageRecords, setUsageRecords] = useState<any[]>([]);
  const [weeklyData, setWeeklyData] = useState<any[]>([]);
  const [accessLogs, setAccessLogs] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [detailModalVisible, setDetailModalVisible] = useState(false);
  const [selectedLog, setSelectedLog] = useState<any>(null);

  useEffect(() => {
    fetchData();
  }, []);

  const fetchData = async () => {
    try {
      const [quotaRes, usageRes, logsRes] = await Promise.all([
        api.get('/api/v1/user/quota'),
        api.get('/api/v1/user/usage'),
        api.get('/api/v1/user/access-logs?detailed=true'),
      ]);

      setQuota(quotaRes.data.data || {});
      const records = usageRes.data.data || [];
      setUsageRecords(records);

      // 转换数据为图表格式
      const chartData = records.map((record: any) => {
        const date = new Date(record.date);
        const weekdays = ['周日', '周一', '周二', '周三', '周四', '周五', '周六'];
        return {
          date: weekdays[date.getDay()],
          requests: record.requests,
        };
      }).reverse(); // 按时间正序
      setWeeklyData(chartData);

      // 设置访问日志
      setAccessLogs(logsRes.data.data || []);
    } catch (err) {
      console.error('Failed to fetch usage data:', err);
    } finally {
      setLoading(false);
    }
  };

  // 格式化字节数为可读格式
  const formatBytes = (bytes: number): string => {
    if (bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
  };

  // 格式化耗时
  const formatDuration = (ms: number): string => {
    if (ms == null) return '-';
    if (ms < 1000) return `${ms}ms`;
    if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`;
    const minutes = Math.floor(ms / 60000);
    const seconds = ((ms % 60000) / 1000).toFixed(1);
    return seconds === '0.0' ? `${minutes}m` : `${minutes}m ${seconds}s`;
  };

  // 获取HTTP方法对应的颜色
  const getMethodColor = (method: string): string => {
    const colorMap: { [key: string]: string } = {
      'GET': 'blue',
      'POST': 'green',
      'PUT': 'orange',
      'DELETE': 'red',
      'PATCH': 'purple',
    };
    return colorMap[method.toUpperCase()] || 'default';
  };

  // 获取状态码对应的颜色
  const getStatusColor = (status: number): string => {
    if (status >= 200 && status < 300) return 'success';
    if (status >= 400) return 'error';
    return 'warning';
  };

  // JSON 格式化辅助函数
  const formatJSON = (jsonString: string): string => {
    if (!jsonString) return '';
    try {
      const parsed = JSON.parse(jsonString);
      return JSON.stringify(parsed, null, 2);
    } catch {
      return jsonString;
    }
  };

  // HTML 转义函数 - 仅转义 XSS 危险字符，保留 JSON 格式
  const escapeHtml = (unsafe: string): string => {
    return unsafe
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;");
  };

  // 访问日志表格列定义
  const accessLogColumns = [
    {
      title: '时间',
      dataIndex: 'timestamp',
      key: 'timestamp',
      width: 180,
      render: (timestamp: string) => {
        const date = new Date(timestamp);
        return date.toLocaleString('zh-CN');
      },
    },
    {
      title: '方法',
      dataIndex: 'method',
      key: 'method',
      width: 100,
      render: (method: string) => (
        <Tag color={getMethodColor(method)}>{method.toUpperCase()}</Tag>
      ),
    },
    {
      title: '路径',
      dataIndex: 'path',
      key: 'path',
      width: 160,
      ellipsis: true,
    },
    {
      title: '模型',
      dataIndex: 'model_name',
      key: 'model_name',
      width: 100,
      render: (model: string) => model ? <Tag color="default">{model}</Tag> : '-',
    },
    {
      title: '来源/客户端',
      dataIndex: 'user_agent',
      key: 'user_agent',
      width: 130,
      ellipsis: true,
      render: (ua: string) => (
        <AntTooltip title={ua}>
          <span>{ua || '-'}</span>
        </AntTooltip>
      ),
    },
    {
      title: '流量(字节)',
      key: 'traffic',
      width: 150,
      render: (_: any, record: any) => (
        <span>
          <span style={{ color: '#52c41a' }}>↑{formatBytes(record.request_bytes || 0)}</span>
          <span style={{ margin: '0 4px' }}>/</span>
          <span style={{ color: '#1890ff' }}>↓{formatBytes(record.response_bytes || 0)}</span>
        </span>
      ),
    },
    {
      title: 'Tokens',
      key: 'tokens',
      width: 150,
      render: (_: any, record: any) => (
        <span>
          <span style={{ color: '#fa8c16' }}>↑{record.input_tokens || 0}</span>
          <span style={{ margin: '0 4px' }}>/</span>
          <span style={{ color: '#722ed1' }}>↓{record.output_tokens || 0}</span>
        </span>
      ),
    },
    {
      title: 'IP地址',
      dataIndex: 'client_ip',
      key: 'client_ip',
      width: 130,
    },
    {
      title: '状态',
      dataIndex: 'status_code',
      key: 'status_code',
      width: 100,
      render: (status: number) => (
        <Tag color={getStatusColor(status)}>{status}</Tag>
      ),
    },
    {
      title: '耗时',
      dataIndex: 'duration_ms',
      key: 'duration_ms',
      width: 80,
      render: (ms: number) => formatDuration(ms),
    },
    {
      title: '操作',
      key: 'action',
      width: 100,
      render: (_: any, record: any) => (
        <Button
          type="link"
          size="small"
          icon={<EyeOutlined />}
          onClick={() => {
            setSelectedLog(record);
            setDetailModalVisible(true);
          }}
        >
          详情
        </Button>
      ),
    },
  ];

  const requestUsagePercent = quota.daily_requests_limit
    ? Math.round((quota.daily_requests_used / quota.daily_requests_limit) * 100)
    : 0;

  return (
    <div>
      <h2 style={{ marginBottom: 24 }}>使用统计</h2>

      {/* 配额概览 */}
      <Row gutter={16} style={{ marginBottom: 24 }}>
        <Col span={12}>
          <Card>
            <Statistic
              title="今日请求次数"
              value={quota.daily_requests_used || 0}
              suffix={`/ ${quota.daily_requests_limit || 0}`}
            />
            <Progress
              percent={requestUsagePercent}
              status={requestUsagePercent > 90 ? 'exception' : 'active'}
              style={{ marginTop: 8 }}
            />
          </Card>
        </Col>
        <Col span={12}>
          <Card>
            <Statistic
              title="可用模型数"
              value={quota.models_allowed?.length || 0}
              suffix="个"
            />
            <div style={{ marginTop: 8 }}>
              {quota.models_allowed?.map((model: string) => (
                <Tag key={model} style={{ margin: '0 4px 4px 0' }}>
                  {model}
                </Tag>
              ))}
            </div>
          </Card>
        </Col>
      </Row>

      {/* 配额详情 */}
      <Card title="配额详情" style={{ marginBottom: 24 }}>
        <Descriptions bordered column={2}>
          <Descriptions.Item label="速率限制">
            {quota.rate_limit} 请求/{quota.rate_window || 60}秒
          </Descriptions.Item>
          <Descriptions.Item label="每日请求限额">
            {quota.daily_requests_limit?.toLocaleString() || '无限制'}
          </Descriptions.Item>
          <Descriptions.Item label="重置时间">
            {quota.reset_time || '每日 00:00'}
          </Descriptions.Item>
        </Descriptions>
      </Card>

      {/* 最近访问记录 */}
      <Card title="最近20次访问" style={{ marginBottom: 24 }}>
        <Table
          dataSource={accessLogs}
          columns={accessLogColumns}
          rowKey={(record: any) => `${record.timestamp}-${record.path}`}
          loading={loading}
          pagination={false}
          scroll={{ y: 400 }}
          size="small"
        />
      </Card>

      {/* 使用趋势图 */}
      <Card title="最近7天使用趋势" style={{ marginBottom: 24 }}>
        <ResponsiveContainer width="100%" height={300}>
          <BarChart data={weeklyData}>
            <CartesianGrid strokeDasharray="3 3" />
            <XAxis dataKey="date" />
            <YAxis />
            <Tooltip />
            <Bar dataKey="requests" name="请求数" fill="#1890ff" />
          </BarChart>
        </ResponsiveContainer>
      </Card>

      {/* 详情弹窗 */}
      <Modal
        title="请求/响应详情"
        open={detailModalVisible}
        onCancel={() => setDetailModalVisible(false)}
        width={900}
        footer={null}
      >
        {selectedLog && (
          <div style={{ maxHeight: '70vh', overflow: 'auto' }}>
            <Card title="请求信息" size="small" style={{ marginBottom: 16 }}>
              <Descriptions column={1} size="small">
                <Descriptions.Item label="Method">{selectedLog.method}</Descriptions.Item>
                <Descriptions.Item label="Path">{selectedLog.path}</Descriptions.Item>
                <Descriptions.Item label="Model">{selectedLog.model_name || '-'}</Descriptions.Item>
                <Descriptions.Item label="Client IP">{selectedLog.client_ip}</Descriptions.Item>
                <Descriptions.Item label="User Agent" style={{ wordBreak: 'break-all' }}>
                  {selectedLog.user_agent}
                </Descriptions.Item>
                <Descriptions.Item label="Tokens" style={{ wordBreak: 'break-all' }}>
                  Input: {selectedLog.input_tokens || 0} / Output: {selectedLog.output_tokens || 0}
                </Descriptions.Item>
                <Descriptions.Item label="Headers">
                  <pre style={{
                    maxHeight: 150,
                    overflow: 'auto',
                    background: '#f5f5f5',
                    padding: 8,
                    borderRadius: 4,
                    fontSize: 12
                  }}>
                    {JSON.stringify(selectedLog.request_headers, null, 2)}
                  </pre>
                </Descriptions.Item>
                <Descriptions.Item label="Request Body">
                  <pre style={{
                    maxHeight: 300,
                    overflow: 'auto',
                    background: '#f5f5f5',
                    padding: 8,
                    borderRadius: 4,
                    fontSize: 12
                  }}>
                    {formatJSON(selectedLog.request_body)}
                  </pre>
                </Descriptions.Item>
              </Descriptions>
            </Card>

            <Card title="响应信息" size="small">
              <Descriptions column={1} size="small">
                <Descriptions.Item label="Status Code">
                  <Tag color={getStatusColor(selectedLog.status_code)}>
                    {selectedLog.status_code}
                  </Tag>
                </Descriptions.Item>
                <Descriptions.Item label="Response Bytes">
                  {formatBytes(selectedLog.response_bytes || 0)}
                </Descriptions.Item>
                <Descriptions.Item label="Headers">
                  <pre style={{
                    maxHeight: 150,
                    overflow: 'auto',
                    background: '#f5f5f5',
                    padding: 8,
                    borderRadius: 4,
                    fontSize: 12
                  }}>
                    {JSON.stringify(selectedLog.response_headers, null, 2)}
                  </pre>
                </Descriptions.Item>
                <Descriptions.Item label="Response Body">
                  <pre style={{
                    maxHeight: 400,
                    overflow: 'auto',
                    background: '#f5f5f5',
                    padding: 8,
                    borderRadius: 4,
                    fontSize: 12
                  }}>
                    {escapeHtml(formatJSON(selectedLog.response_body))}
                  </pre>
                </Descriptions.Item>
              </Descriptions>
            </Card>
          </div>
        )}
      </Modal>
    </div>
  );
};

export default UsageStats;
