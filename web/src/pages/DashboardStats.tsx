import React, { useState, useEffect } from 'react';
import {
  Row,
  Col,
  Card,
  Statistic,
  Table,
  Empty,
  message,
} from 'antd';
import {
  UserOutlined,
  CloudServerOutlined,
  ThunderboltOutlined,
  HistoryOutlined,
} from '@ant-design/icons';
import {
  Bar,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
  PieChart,
  Pie,
  Cell,
  Legend,
  ComposedChart,
} from 'recharts';
import api from '../api';

interface DashboardData {
  summary: {
    total_users: number;
    total_models: number;
    today_requests: number;
    today_tokens: number;
  };
  hourly_stats: {
    hour: string;
    requests: number;
    input_tokens: number;
    output_tokens: number;
    total_tokens: number;
  }[];
  top_users: {
    user_id: string;
    username: string;
    request_count: number;
    input_tokens: number;
    output_tokens: number;
  }[];
  model_stats: {
    model_id: string;
    request_count: number;
    input_tokens: number;
    output_tokens: number;
  }[];
  department_stats: {
    department: string;
    request_count: number;
    input_tokens: number;
    output_tokens: number;
  }[];
}

const PIE_COLORS = ['#0088FE', '#00C49F', '#FFBB28', '#FF8042', '#8884d8', '#82ca9d', '#ffc658', '#8dd1e1', '#a4de6c', '#d0ed57'];

const DashboardStats: React.FC = () => {
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<DashboardData | null>(null);

  const fetchStats = async () => {
    try {
      const [statsRes, hourlyRes, topUsersRes, modelRes, deptRes] = await Promise.all([
        api.get('/api/v1/dashboard/stats'),
        api.get('/api/v1/dashboard/hourly'),
        api.get('/api/v1/dashboard/top-users'),
        api.get('/api/v1/dashboard/models'),
        api.get('/api/v1/dashboard/departments'),
      ]);
      const stats = statsRes.data.data || {};
      setData({
        summary: {
          total_users: stats.total_users || 0,
          total_models: stats.department_count || 0,
          today_requests: stats.today_total_requests || 0,
          today_tokens: (stats.today_input_tokens || 0) + (stats.today_output_tokens || 0),
        },
        hourly_stats: (hourlyRes.data.data || []).map((h: any) => ({
          ...h,
          total_tokens: (h.input_tokens || 0) + (h.output_tokens || 0),
        })),
        top_users: (topUsersRes.data.data || []).map((u: any) => ({
          user_id: u.user_id,
          username: u.name || u.user_id,
          request_count: u.request_count,
          input_tokens: u.input_tokens || 0,
          output_tokens: u.output_tokens || 0,
        })),
        model_stats: modelRes.data.data || [],
        department_stats: deptRes.data.data || [],
      });
    } catch (error: any) {
      message.error('获取统计数据失败: ' + (error.response?.data?.error || error.message));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchStats();
    // 每 5 分钟刷新一次
    const timer = setInterval(fetchStats, 5 * 60 * 1000);
    return () => clearInterval(timer);
  }, []);

  if (loading) {
    return <Card loading={true} />;
  }

  if (!data) {
    return <Empty description="无法加载数据" />;
  }

  const { summary, hourly_stats: hourlyStats, top_users: topUsers, model_stats: modelStats, department_stats: departmentStats } = data;

  const formatTokens = (num: number) => {
    if (num >= 1000000) {
      return (num / 1000000).toFixed(2) + 'M';
    }
    if (num >= 1000) {
      return (num / 1000).toFixed(1) + 'K';
    }
    return num.toString();
  };

  const topUserColumns = [
    { title: '用户名', dataKey: 'username', key: 'username', render: (_: any, record: any) => record.username || record.user_id },
    { title: '请求数', dataIndex: 'request_count', key: 'request_count', sorter: (a: any, b: any) => a.request_count - b.request_count },
    { title: 'Token 消耗', key: 'total_tokens', render: (_: any, record: any) => formatTokens((record.input_tokens || 0) + (record.output_tokens || 0)), sorter: (a: any, b: any) => ((a.input_tokens || 0) + (a.output_tokens || 0)) - ((b.input_tokens || 0) + (b.output_tokens || 0)) },
  ];

  const modelTokenColumns = [
    { title: '模型', dataIndex: 'model_id', key: 'model_id' },
    { title: '请求', dataIndex: 'request_count', key: 'request_count' },
    { title: '总 Token', key: 'total_tokens', render: (_: any, record: any) => formatTokens((record.input_tokens || 0) + (record.output_tokens || 0)) },
  ];

  const departmentColumns = [
    { title: '部门', dataIndex: 'department', key: 'department', render: (text: string) => text || '未设置' },
    { title: '请求数', dataIndex: 'request_count', key: 'request_count' },
    { title: 'Token 消耗', key: 'tokens', render: (_: any, record: any) => formatTokens((record.input_tokens || 0) + (record.output_tokens || 0)) },
  ];

  return (
    <div className="dashboard-stats">
      <Row gutter={16} style={{ marginBottom: 24 }}>
        <Col xs={24} sm={12} lg={6}>
          <Card bordered={false} hoverable>
            <Statistic
              title="用户总数"
              value={summary.total_users}
              prefix={<UserOutlined style={{ color: '#1890ff' }} />}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <Card bordered={false} hoverable>
            <Statistic
              title="已接入模型"
              value={summary.total_models}
              prefix={<CloudServerOutlined style={{ color: '#52c41a' }} />}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <Card bordered={false} hoverable>
            <Statistic
              title="今日总请求"
              value={summary.today_requests}
              prefix={<ThunderboltOutlined style={{ color: '#faad14' }} />}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <Card bordered={false} hoverable>
            <Statistic
              title="今日 Token 消耗"
              value={formatTokens(summary.today_tokens)}
              prefix={<HistoryOutlined style={{ color: '#f5222d' }} />}
            />
          </Card>
        </Col>
      </Row>

      {/* 24小时趋势 + TOP10用户 */}
      <Row gutter={16} style={{ marginBottom: 24 }}>
        <Col xs={24} lg={14}>
          <Card title="最近24小时趋势">
            {hourlyStats.length > 0 && hourlyStats.some(s => s.requests > 0 || s.total_tokens > 0) ? (
              <ResponsiveContainer width="100%" height={300}>
                <ComposedChart data={hourlyStats}>
                  <CartesianGrid strokeDasharray="3 3" />
                  <XAxis dataKey="hour" tick={{ fontSize: 12 }} />
                  <YAxis yAxisId="left" orientation="left" stroke="#1890ff" label={{ value: '请求数', angle: -90, position: 'insideLeft', offset: 10 }} />
                  <YAxis yAxisId="right" orientation="right" stroke="#f5222d" tickFormatter={(v: number) => formatTokens(v)} label={{ value: 'Token', angle: 90, position: 'insideRight', offset: 10 }} />
                  <Tooltip
                    formatter={(value: any, name: any) => {
                      if (name === '请求数') return [`${value ?? 0}`, '请求数'];
                      return [formatTokens(Number(value ?? 0)), 'Token 总量'];
                    }}
                    labelFormatter={(label: any) => `${label}`}
                  />
                  <Legend />
                  <Bar yAxisId="left" dataKey="requests" fill="#1890ff" name="请求数" />
                  <Line yAxisId="right" type="monotone" dataKey="total_tokens" stroke="#f5222d" name="Token 总量" dot={false} strokeWidth={2} />
                </ComposedChart>
              </ResponsiveContainer>
            ) : (
              <Empty description="暂无数据" style={{ padding: '60px 0' }} />
            )}
          </Card>
        </Col>
        <Col xs={24} lg={10}>
          <Card title="今日 TOP10 用户">
            {topUsers.length > 0 && topUsers.some(u => u.request_count > 0) ? (
              <Table
                dataSource={topUsers.filter(u => u.request_count > 0).slice(0, 10)}
                columns={topUserColumns}
                rowKey="user_id"
                pagination={false}
                size="small"
                scroll={{ y: 300 }}
              />
            ) : (
              <Empty description="暂无数据" style={{ padding: '60px 0' }} />
            )}
          </Card>
        </Col>
      </Row>

      {/* 模型 Token 统计 */}
      <Row gutter={16} style={{ marginBottom: 24 }}>
        <Col xs={24}>
          <Card title="今日模型 Token 消耗">
            {modelStats.length > 0 && modelStats.some(s => s.request_count > 0) ? (
              <Row gutter={16}>
                <Col xs={24} lg={12}>
                  <Table
                    dataSource={modelStats.filter(s => s.request_count > 0)}
                    columns={modelTokenColumns}
                    rowKey="model_id"
                    pagination={false}
                    size="small"
                    scroll={{ y: 250 }}
                  />
                </Col>
                <Col xs={24} lg={12}>
                  <ResponsiveContainer width="100%" height={260}>
                    <PieChart>
                      <Pie
                        data={modelStats.filter(s => (s.input_tokens || 0) + (s.output_tokens || 0) > 0).map(s => ({
                          ...s,
                          total_tokens: (s.input_tokens || 0) + (s.output_tokens || 0),
                        }))}
                        cx="50%"
                        cy="50%"
                        innerRadius={50}
                        outerRadius={90}
                        paddingAngle={2}
                        dataKey="total_tokens"
                        nameKey="model_id"
                        label={({ payload, percent }: any) =>
                          `${payload?.model_id || ''} ${(Number(percent || 0) * 100).toFixed(0)}%`
                        }
                      >
                        {modelStats.map((_entry, index) => (
                          <Cell
                            key={`cell-${index}`}
                            fill={PIE_COLORS[index % PIE_COLORS.length]}
                          />
                        ))}
                      </Pie>
                      <Tooltip
                        formatter={(value: any, _name: any, props: any) => {
                          return [`${formatTokens(Number(value || 0))} Tokens`, props.payload?.model_id || ''];
                        }}
                      />
                      <Legend />
                    </PieChart>
                  </ResponsiveContainer>
                </Col>
              </Row>
            ) : (
              <Empty description="暂无数据" style={{ padding: '40px 0' }} />
            )}
          </Card>
        </Col>
      </Row>

      {/* 部门统计 + 模型请求分布 */}
      <Row gutter={16}>
        <Col xs={24} lg={12}>
          <Card title="部门使用统计">
            {departmentStats.length > 0 && departmentStats.some(s => s.request_count > 0) ? (
              <Table
                dataSource={departmentStats.filter(s => s.request_count > 0)}
                columns={departmentColumns}
                rowKey="department"
                pagination={false}
                size="small"
                scroll={{ y: 300 }}
              />
            ) : (
              <Empty description="暂无数据" style={{ padding: '60px 0' }} />
            )}
          </Card>
        </Col>
        <Col xs={24} lg={12}>
          <Card title="模型请求分布">
            {modelStats.length > 0 && modelStats.some(s => s.request_count > 0) ? (
              <ResponsiveContainer width="100%" height={300}>
                <PieChart>
                  <Pie
                    data={modelStats.filter(s => s.request_count > 0)}
                    cx="50%"
                    cy="50%"
                    innerRadius={60}
                    outerRadius={100}
                    paddingAngle={2}
                    dataKey="request_count"
                    nameKey="model_id"
                    label={({ payload, percent }: any) =>
                      `${payload?.model_id || ''} ${(Number(percent || 0) * 100).toFixed(0)}%`
                    }
                  >
                    {modelStats
                      .filter(s => s.request_count > 0)
                      .map((_entry, index) => (
                        <Cell
                          key={`cell-${index}`}
                          fill={PIE_COLORS[index % PIE_COLORS.length]}
                        />
                      ))}
                  </Pie>
                  <Tooltip
                    formatter={(value: any, _name: any, props: any) => {
                      const total = modelStats.reduce((sum, s) => sum + s.request_count, 0);
                      const percent = total > 0 ? ((Number(value || 0) / total) * 100).toFixed(1) : '0';
                      return [`${value ?? 0} (${percent}%)`, props.payload?.model_id || ''];
                    }}
                  />
                  <Legend />
                </PieChart>
              </ResponsiveContainer>
            ) : (
              <Empty description="暂无数据" style={{ padding: '60px 0' }} />
            )}
          </Card>
        </Col>
      </Row>
    </div>
  );
};

export default DashboardStats;
