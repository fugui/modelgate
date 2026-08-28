import React, { useState, useEffect } from 'react';
import {
  Row,
  Col,
  Card,
  Tag,
  Select,
  Space,
  Switch,
  Empty,
  message,
} from 'antd';
import {
  UserOutlined,
  CloudServerOutlined,
  ThunderboltOutlined,
  CommentOutlined,
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
import { MetricCard } from '../components/dashboard/MetricCard';
import { TopList } from '../components/dashboard/TopList';

interface DashboardData {
  summary: {
    total_users: number;
    peak_concurrency: number;
    today_requests: number;
    today_tokens: number;
    today_sessions: number;
    today_session_requests: number;
    today_no_session_requests: number;
    today_session_ratio: number;
    today_avg_session_depth: number;
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
  model_stats: {
    model_id: string;
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

  backend_metrics: {
    time_label: string;
    [backendId: string]: number | string; // dynamic keys: backend_xxx for latency values
  }[];
  backend_ids: string[];
}



const DashboardStats: React.FC = () => {
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<DashboardData | null>(null);
  const [selectedBackends, setSelectedBackends] = useState<string[]>([]);
  const [showLatency, setShowLatency] = useState<boolean>(true);

  const fetchStats = async () => {
    try {
      const [statsRes, hourlyRes, modelStatsRes, metricsRes, backendMetricsRes] = await Promise.all([
        api.get('/api/v1/dashboard/stats'),
        api.get('/api/v1/dashboard/hourly'),
        api.get('/api/v1/dashboard/models'),
        api.get('/api/v1/dashboard/metrics'),
        api.get('/api/v1/dashboard/backend-metrics'),
      ]);
      const stats = statsRes.data.data || {};
      setData({
        summary: {
          total_users: stats.total_users || 0,
          peak_concurrency: stats.peak_concurrency || 0,
          today_requests: stats.today_total_requests || 0,
          today_tokens: (stats.today_input_tokens || 0) + (stats.today_output_tokens || 0),
          today_sessions: stats.today_sessions || 0,
          today_session_requests: stats.today_session_requests || 0,
          today_no_session_requests: stats.today_no_session_requests || 0,
          today_session_ratio: stats.today_session_ratio || 0,
          today_avg_session_depth: stats.today_avg_session_depth || 0,
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
        model_stats: (modelStatsRes.data.data || []).map((m: any) => ({
          model_id: m.model_id,
          request_count: m.request_count || 0,
          input_tokens: m.input_tokens || 0,
          output_tokens: m.output_tokens || 0,
        })),

        metrics_history: (metricsRes.data.data || []).map((m: any) => ({
          ...m,
          avg_latency_ms: Math.round((m.avg_latency_ms || 0) * 100) / 100,
        })),

        // 处理 backend metrics: 将 { backendId: [{timestamp, time_label, avg_latency_ms}] } 转为统一时间轴
        ...(() => {
          const raw: Record<string, { timestamp: number; time_label: string; avg_latency_ms: number; request_count: number }[]> = backendMetricsRes.data.data || {};
          const backendIds = Object.keys(raw);
          if (backendIds.length === 0) {
            return { backend_metrics: [] as any[], backend_ids: [] as string[] };
          }
          // 收集所有时间戳并按时间排序（解决跨天排序问题）
          const timeMap = new Map<number, string>(); // timestamp -> time_label
          backendIds.forEach(id => raw[id]?.forEach(s => {
            if (!timeMap.has(s.timestamp)) timeMap.set(s.timestamp, s.time_label);
          }));
          const timestamps = Array.from(timeMap.keys()).sort((a, b) => a - b);
          // 构建每个时间点的行数据
          const chartData = timestamps.map(ts => {
            const row: any = { time_label: timeMap.get(ts) };
            backendIds.forEach(id => {
              const snap = raw[id]?.find(s => s.timestamp === ts);
              row[`${id}_latency`] = snap ? Math.round(snap.avg_latency_ms * 100) / 100 : null;
              row[`${id}_requests`] = snap ? snap.request_count : 0;
            });
            return row;
          });
          return { backend_metrics: chartData, backend_ids: backendIds };
        })(),
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

  const { summary, hourly_stats: hourlyStats, model_stats: modelStats, metrics_history: metricsHistory, backend_metrics: backendMetrics, backend_ids: backendIds } = data;
  const visibleBackends = selectedBackends.length > 0 ? selectedBackends : backendIds;

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

  const formatLatency = (ms: number | null | undefined) => {
    if (ms == null) return '-';
    if (ms < 1000) return `${Math.round(ms)} ms`;
    const seconds = ms / 1000;
    if (seconds < 60) return `${seconds.toFixed(2)} s`;
    const minutes = Math.floor(seconds / 60);
    const remainingSeconds = Math.round(seconds % 60);
    return `${minutes}m ${remainingSeconds}s`;
  };

  const renderTokens = (_: any, record: any) => (
    <span>
      <span style={{ color: '#fa8c16' }}>↑{formatTokens(record.input_tokens || 0)}</span>
      <span style={{ margin: '0 4px' }}>/</span>
      <span style={{ color: '#722ed1' }}>↓{formatTokens(record.output_tokens || 0)}</span>
    </span>
  );

  const totalModelRequests = modelStats.reduce((sum, item) => sum + (item.request_count || 0), 0);

  const modelColumns = [
    {
      title: '模型',
      dataIndex: 'model_id',
      key: 'model_id',
      render: (text: string) => (
        <span style={{ fontFamily: 'monospace', fontWeight: 600, color: '#1890ff' }}>
          {text}
        </span>
      ),
    },
    {
      title: '请求数',
      dataIndex: 'request_count',
      key: 'request_count',
      sorter: (a: any, b: any) => (a.request_count || 0) - (b.request_count || 0),
    },
    {
      title: '请求占比',
      key: 'percentage',
      render: (_: any, record: any) => {
        const pct = totalModelRequests > 0 ? ((record.request_count || 0) / totalModelRequests) * 100 : 0;
        return (
          <div style={{ display: 'flex', alignItems: 'center', width: '100%' }}>
            <span style={{ width: '45px', marginRight: '8px' }}>{pct.toFixed(1)}%</span>
            <div style={{ flex: 1, height: '6px', backgroundColor: '#f5f5f5', borderRadius: '3px', overflow: 'hidden' }}>
              <div style={{ width: `${pct}%`, height: '100%', backgroundColor: '#1890ff', borderRadius: '3px' }} />
            </div>
          </div>
        );
      },
      sorter: (a: any, b: any) => (a.request_count || 0) - (b.request_count || 0),
    },
    {
      title: 'Tokens (输入/输出)',
      key: 'tokens',
      render: renderTokens,
      sorter: (a: any, b: any) => ((a.input_tokens || 0) + (a.output_tokens || 0)) - ((b.input_tokens || 0) + (b.output_tokens || 0)),
    },
  ];





  return (
    <div className="dashboard-stats">
      <Row gutter={16} style={{ marginBottom: 24 }}>
        <Col xs={24} sm={12} lg={6}>
          <MetricCard
            title="用户总数"
            value={summary.total_users}
            prefix={<UserOutlined style={{ color: '#1890ff' }} />}
            valueStyle={{ color: '#1890ff' }}
          />
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <MetricCard
            title="今日最高并发"
            value={summary.peak_concurrency}
            prefix={<CloudServerOutlined style={{ color: '#52c41a' }} />}
            valueStyle={{ color: '#52c41a' }}
          />
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <MetricCard
            title="今日总请求"
            value={summary.today_requests}
            prefix={<ThunderboltOutlined style={{ color: '#faad14' }} />}
            valueStyle={{ color: '#faad14' }}
          />
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <MetricCard
            title="今日活跃会话"
            value={summary.today_sessions}
            suffix={
              summary.today_sessions > 0
                ? `(均深 ${summary.today_avg_session_depth.toFixed(1)})`
                : ''
            }
            prefix={<CommentOutlined style={{ color: '#13c2c2' }} />}
            valueStyle={{ color: '#13c2c2' }}
          />
        </Col>
      </Row>

      {/* 今日流量构成透视 */}
      <Card size="small" style={{ marginBottom: 24 }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', flexWrap: 'wrap', gap: 12 }}>
          <Space>
            <span style={{ fontWeight: 'bold' }}>今日请求构成:</span>
            <Tag color="cyan">
              会话内请求: {summary.today_session_requests.toLocaleString()} 次 ({((summary.today_session_ratio || 0) * 100).toFixed(1)}%)
            </Tag>
            <Tag color="orange">
              无会话独立请求: {summary.today_no_session_requests.toLocaleString()} 次 ({((1 - (summary.today_session_ratio || 0)) * 100).toFixed(1)}%)
            </Tag>
          </Space>
          <span style={{ color: '#888', fontSize: 13 }}>
            今日独立会话: <b style={{ color: '#13c2c2' }}>{summary.today_sessions}</b> 个 | 今日 Token 消耗: <b style={{ color: '#f5222d' }}>{formatTokens(summary.today_tokens)}</b>
          </span>
        </div>
      </Card>

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
                    itemSorter={(item: any) => (item.name === 'Token 总量' ? -1 : 1)}
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
          {modelStats.length > 0 && modelStats.some(m => m.request_count > 0) ? (
            <TopList
              title="今日模型请求 Token"
              dataSource={modelStats.filter(m => m.request_count > 0)}
              columns={modelColumns as any}
              rowKey="model_id"
              scroll={{ y: 300 }}
            />
          ) : (
            <Card title="今日模型请求 Token">
              <Empty description="暂无数据" style={{ padding: '60px 0' }} />
            </Card>
          )}
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
                      if (name === '平均时延') return [formatLatency(value as number), '平均时延'];
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

      {/* 后端时延对比 */}
      {backendMetrics.length > 0 && (
        <Row gutter={16} style={{ marginBottom: 24 }}>
          <Col xs={24}>
            <Card 
              title="后端请求数 & 平均时延对比（最近24小时，5分钟粒度）"
              extra={
                <Space>
                  <Select
                    mode="multiple"
                    allowClear
                    placeholder="选择模型展示（默认全部）"
                    style={{ minWidth: 200, maxWidth: 400 }}
                    value={selectedBackends}
                    onChange={setSelectedBackends}
                    maxTagCount="responsive"
                  >
                    {backendIds.map(id => (
                      <Select.Option key={id} value={id}>{id}</Select.Option>
                    ))}
                  </Select>
                  <Switch checked={showLatency} onChange={setShowLatency} checkedChildren="时延" unCheckedChildren="时延" />
                </Space>
              }
            >
              <ResponsiveContainer width="100%" height={300}>
                <ComposedChart data={backendMetrics}>
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
                    label={{ value: '请求数', angle: -90, position: 'insideLeft', offset: 10 }}
                  />
                  {showLatency && (
                    <YAxis
                      yAxisId="right"
                      orientation="right"
                      stroke="#fa8c16"
                      label={{ value: '时延(ms)', angle: 90, position: 'insideRight', offset: 10 }}
                    />
                  )}
                  <Tooltip
                    content={(props: any) => {
                      const { active, payload, label } = props;
                      if (!active || !payload || !payload.length) return null;

                      const models = new Map<string, { requests?: number; latency?: number; color: string }>();

                      payload.forEach((item: any) => {
                        let id = item.name;
                        let isLatency = false;
                        if (typeof item.name === 'string' && item.name.endsWith(' (时延)')) {
                           id = item.name.replace(' (时延)', '');
                           isLatency = true;
                        }

                        if (!models.has(id)) {
                          models.set(id, { color: item.color });
                        }
                        const modelData = models.get(id)!;
                        if (isLatency) {
                          modelData.latency = item.value;
                        } else {
                          modelData.requests = item.value;
                        }
                      });

                      return (
                        <div style={{ backgroundColor: 'rgba(255, 255, 255, 0.95)', border: '1px solid #d9d9d9', padding: '10px', borderRadius: '4px', boxShadow: '0 2px 8px rgba(0, 0, 0, 0.15)' }}>
                          <p style={{ margin: 0, fontWeight: 'bold', marginBottom: '8px', color: '#333' }}>时间: {label}</p>
                          {Array.from(models.entries()).map(([id, data]) => (
                            <div key={id} style={{ display: 'flex', alignItems: 'center', marginBottom: '4px', fontSize: '13px' }}>
                              <span style={{ display: 'inline-block', minWidth: '8px', height: '8px', backgroundColor: data.color, borderRadius: '50%', marginRight: '8px' }}></span>
                              <span style={{ color: '#555', marginRight: '8px', fontWeight: 500 }}>{id}:</span>
                              <span style={{ color: '#333' }}>
                                {data.requests ?? 0} 请求
                                {showLatency && data.latency != null && (
                                  <span style={{ color: '#888', marginLeft: '4px' }}>
                                    ({formatLatency(data.latency)})
                                  </span>
                                )}
                              </span>
                            </div>
                          ))}
                        </div>
                      );
                    }}
                  />
                  <Legend />
                  {visibleBackends.map((id) => {
                    const colorIndex = backendIds.indexOf(id);
                    return (
                      <Bar
                        key={`${id}_requests`}
                        yAxisId="left"
                        dataKey={`${id}_requests`}
                        fill={chartColors[colorIndex % chartColors.length]}
                        fillOpacity={0.4}
                        name={id}
                        stackId="requests"
                      />
                    );
                  })}
                  {showLatency && visibleBackends.map((id) => {
                    const colorIndex = backendIds.indexOf(id);
                    return (
                      <Line
                        key={`${id}_latency`}
                        yAxisId="right"
                        type="monotone"
                        dataKey={`${id}_latency`}
                        stroke={chartColors[colorIndex % chartColors.length]}
                        name={`${id} (时延)`}
                        dot={false}
                        strokeWidth={2}
                        connectNulls
                        legendType="none"
                      />
                    );
                  })}
                </ComposedChart>
              </ResponsiveContainer>
            </Card>
          </Col>
        </Row>
      )}


    </div>
  );
};

export default DashboardStats;
