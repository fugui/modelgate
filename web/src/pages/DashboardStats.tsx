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
  Area,
  Bar,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
  Legend,
  ComposedChart,
} from 'recharts';
import api from '../api';

interface DashboardData {
  summary: {
    total_users: number;
    peak_concurrency: number;
    today_requests: number;
    today_tokens: number;
  };
  hourly_stats: {
    hour: string;
    requests: number;
    input_tokens: number;
    output_tokens: number;
    total_tokens: number;
    models?: {
      [key: string]: {
        requests: number;
        input_tokens: number;
        output_tokens: number;
      };
    };
    [key: string]: any;
  }[];
  top_users: {
    user_id: string;
    username: string;
    request_count: number;
    input_tokens: number;
    output_tokens: number;
  }[];

  metrics_history: {
    timestamp: string;
    time_label: string;
    concurrency: number;
    avg_latency_ms: number;
    request_count: number;
  }[];
}



const DashboardStats: React.FC = () => {
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<DashboardData | null>(null);

  const fetchStats = async () => {
    try {
      const [statsRes, hourlyRes, topUsersRes, metricsRes] = await Promise.all([
        api.get('/api/v1/dashboard/stats'),
        api.get('/api/v1/dashboard/hourly'),
        api.get('/api/v1/dashboard/top-users'),
        api.get('/api/v1/dashboard/metrics'),
      ]);
      const stats = statsRes.data.data || {};
      setData({
        summary: {
          total_users: stats.total_users || 0,
          peak_concurrency: stats.peak_concurrency || 0,
          today_requests: stats.today_total_requests || 0,
          today_tokens: (stats.today_input_tokens || 0) + (stats.today_output_tokens || 0),
        },
        hourly_stats: (hourlyRes.data.data || []).map((h: any) => {
          const stat: any = {
            ...h,
            total_tokens: (h.input_tokens || 0) + (h.output_tokens || 0),
          };
          if (h.models) {
            Object.keys(h.models).forEach((modelId) => {
              stat[`model_${modelId}_requests`] = h.models[modelId].requests;
            });
          }
          return stat;
        }),
        top_users: (topUsersRes.data.data || []).map((u: any) => ({
          user_id: u.user_id,
          username: u.name || u.user_id,
          request_count: u.request_count,
          input_tokens: u.input_tokens || 0,
          output_tokens: u.output_tokens || 0,
        })),

        metrics_history: (metricsRes.data.data || []).map((m: any) => ({
          ...m,
          avg_latency_ms: Math.round((m.avg_latency_ms || 0) * 100) / 100,
        })),
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

  const { summary, hourly_stats: hourlyStats, top_users: topUsers, metrics_history: metricsHistory } = data;

  const uniqueModels = Array.from(
    new Set(hourlyStats.flatMap((stat) => (stat.models ? Object.keys(stat.models) : [])))
  );
  const chartColors = ['#1890ff', '#52c41a', '#faad14', '#f5222d', '#722ed1', '#eb2f96', '#13c2c2', '#fa8c16'];

  const formatTokens = (num: number) => {
    if (num >= 1000000) {
      return (num / 1000000).toFixed(2) + 'M';
    }
    if (num >= 1000) {
      return (num / 1000).toFixed(1) + 'K';
    }
    return num.toString();
  };

  const renderTokens = (_: any, record: any) => (
    <span>
      <span style={{ color: '#fa8c16' }}>↑{formatTokens(record.input_tokens || 0)}</span>
      <span style={{ margin: '0 4px' }}>/</span>
      <span style={{ color: '#722ed1' }}>↓{formatTokens(record.output_tokens || 0)}</span>
    </span>
  );

  const topUserColumns = [
    { title: '用户名', dataKey: 'username', key: 'username', render: (_: any, record: any) => record.username || record.user_id },
    { title: '请求数', dataIndex: 'request_count', key: 'request_count', sorter: (a: any, b: any) => a.request_count - b.request_count },
    { title: 'Tokens', key: 'total_tokens', render: renderTokens, sorter: (a: any, b: any) => ((a.input_tokens || 0) + (a.output_tokens || 0)) - ((b.input_tokens || 0) + (b.output_tokens || 0)) },
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
              title="今日最高并发"
              value={summary.peak_concurrency}
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
                      if (name === '请求数' || name === '请求总数') return [`${value ?? 0}`, '请求总数'];
                      if (name === 'Token 总量') return [formatTokens(Number(value ?? 0)), 'Token 总量'];
                      return [`${value ?? 0}`, `模型: ${name}`];
                    }}
                    labelFormatter={(label: any) => `${label}`}
                  />
                  <Legend />
                  {uniqueModels.length > 0 ? (
                    uniqueModels.map((modelId, index) => (
                      <Bar
                        key={modelId}
                        yAxisId="left"
                        dataKey={`model_${modelId}_requests`}
                        stackId="requests"
                        fill={chartColors[index % chartColors.length]}
                        name={modelId}
                      />
                    ))
                  ) : (
                    <Bar yAxisId="left" dataKey="requests" fill="#1890ff" name="请求数" />
                  )}
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

      {/* 并发数 & 响应时延 */}
      <Row gutter={16} style={{ marginBottom: 24 }}>
        <Col xs={24}>
          <Card title="并发请求 & 响应时延（最近24小时，5分钟粒度）">
            {metricsHistory.length > 0 ? (
              <ResponsiveContainer width="100%" height={300}>
                <ComposedChart data={metricsHistory}>
                  <CartesianGrid strokeDasharray="3 3" />
                  <XAxis
                    dataKey="time_label"
                    tick={{ fontSize: 11 }}
                    interval={11}
                  />
                  <YAxis
                    yAxisId="left"
                    orientation="left"
                    stroke="#1890ff"
                    label={{ value: '并发数', angle: -90, position: 'insideLeft', offset: 10 }}
                  />
                  <YAxis
                    yAxisId="right"
                    orientation="right"
                    stroke="#fa8c16"
                    label={{ value: '时延(ms)', angle: 90, position: 'insideRight', offset: 10 }}
                  />
                  <Tooltip
                    formatter={(value: any, name: any) => {
                      if (name === '并发数') return [`${value ?? 0}`, '并发数'];
                      if (name === '平均时延') return [`${value ?? 0} ms`, '平均时延'];
                      return [value, name];
                    }}
                    labelFormatter={(label: any) => `时间: ${label}`}
                  />
                  <Legend />
                  <Area
                    yAxisId="left"
                    type="monotone"
                    dataKey="concurrency"
                    fill="#1890ff"
                    fillOpacity={0.3}
                    stroke="#1890ff"
                    name="并发数"
                    strokeWidth={2}
                  />
                  <Line
                    yAxisId="right"
                    type="monotone"
                    dataKey="avg_latency_ms"
                    stroke="#fa8c16"
                    name="平均时延"
                    dot={false}
                    strokeWidth={2}
                  />
                </ComposedChart>
              </ResponsiveContainer>
            ) : (
              <Empty description="暂无数据（系统启动后每5分钟采样一次）" style={{ padding: '60px 0' }} />
            )}
          </Card>
        </Col>
      </Row>


    </div>
  );
};

export default DashboardStats;
