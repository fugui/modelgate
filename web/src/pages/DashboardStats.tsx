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
  HistoryOutlined,
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
import { TopList } from '../components/dashboard/TopList';

interface DashboardData {
  summary: {
    total_users: number;
    active_users: number;
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
    timestamp?: number;
    [key: string]: any;
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
          active_users: stats.active_users || 0,
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
            return { backend_metrics: [], backend_ids: [] };
          }

          // 收集所有时间标签并保持顺序
          const timeLabelMap = new Map<string, { timestamp: number; time_label: string; [key: string]: any }>();
          backendIds.forEach((backendId) => {
            (raw[backendId] || []).forEach((item) => {
              if (!timeLabelMap.has(item.time_label)) {
                timeLabelMap.set(item.time_label, {
                  timestamp: item.timestamp,
                  time_label: item.time_label,
                });
              }
              const point = timeLabelMap.get(item.time_label)!;
              point[`${backendId}_latency`] = Math.round(item.avg_latency_ms * 100) / 100;
              point[`${backendId}_requests`] = item.request_count;
            });
          });

          // 按时间戳排序
          const backendMetrics = Array.from(timeLabelMap.values()).sort(
            (a, b) => a.timestamp - b.timestamp
          );

          return { backend_metrics: backendMetrics, backend_ids: backendIds };
        })(),
      });
    } catch (err) {
      console.error('Failed to fetch dashboard stats:', err);
      message.error('获取统计数据失败');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchStats();
    // 每30秒自动刷新一次
    const interval = setInterval(fetchStats, 30000);
    return () => clearInterval(interval);
  }, []);

  if (loading && !data) {
    return (
      <div style={{ textAlign: 'center', padding: '100px 0' }}>
        <p>加载数据中...</p>
      </div>
    );
  }

  if (!data) {
    return (
      <Empty description="暂无数据" style={{ padding: '100px 0' }} />
    );
  }

  const { summary, hourly_stats: hourlyStats, model_stats: modelStats, metrics_history: metricsHistory, backend_metrics: backendMetrics, backend_ids: backendIds } = data;
  const visibleBackends: string[] = selectedBackends.length > 0 ? selectedBackends : backendIds;

  const formatTokens = (tokens: number) => {
    if (tokens >= 1000000) {
      return `${(tokens / 1000000).toFixed(1)}M`;
    }
    if (tokens >= 1000) {
      return `${(tokens / 1000).toFixed(1)}K`;
    }
    return tokens.toLocaleString();
  };

  const formatLatency = (ms: number) => {
    if (ms >= 1000) {
      return `${(ms / 1000).toFixed(2)}s`;
    }
    return `${ms}ms`;
  };

  // 格式化 Token 显示：将数量转换为紧凑的 K/M 格式
  const formatTokensCompact = (tokens: number): string => {
    if (tokens >= 1_000_000) {
      return `${(tokens / 1_000_000).toFixed(1)}M`;
    }
    if (tokens >= 1_000) {
      return `${(tokens / 1_000).toFixed(1)}K`;
    }
    return tokens.toString();
  };

  // 自定义渲染 Token 柱状单元格：展示输入/输出细分和总计标签
  const renderTokens = (_: any, record: any) => {
    const input = record.input_tokens || 0;
    const output = record.output_tokens || 0;
    const total = input + output;

    if (total === 0) {
      return <span style={{ color: '#bbb' }}>-</span>;
    }

    const inputPercent = Math.round((input / total) * 100);
    const outputPercent = 100 - inputPercent;

    return (
      <div style={{ minWidth: 160 }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 12, marginBottom: 2 }}>
          <span style={{ color: '#1890ff' }}>入: {formatTokensCompact(input)} ({inputPercent}%)</span>
          <span style={{ color: '#52c41a' }}>出: {formatTokensCompact(output)} ({outputPercent}%)</span>
        </div>
        <div style={{ height: 6, display: 'flex', borderRadius: 3, overflow: 'hidden', background: '#f0f0f0' }}>
          <div style={{ width: `${inputPercent}%`, background: '#1890ff' }} />
          <div style={{ width: `${outputPercent}%`, background: '#52c41a' }} />
        </div>
        <div style={{ fontSize: 11, color: '#888', textAlign: 'right', marginTop: 1 }}>
          总量: {formatTokensCompact(total)}
        </div>
      </div>
    );
  };

  const modelColumns = [
    {
      title: '模型名称',
      dataIndex: 'model_id',
      key: 'model_id',
      render: (text: string) => <Tag color="blue">{text}</Tag>,
    },
    {
      title: '请求数',
      dataIndex: 'request_count',
      key: 'request_count',
      render: (count: number) => {
        const total = summary.today_requests || 1;
        const percent = Math.round((count / total) * 100);
        return (
          <div style={{ minWidth: 120 }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 12, marginBottom: 2 }}>
              <span>{count.toLocaleString()} 次</span>
              <span style={{ color: '#888' }}>{percent}%</span>
            </div>
            <div style={{ height: 4, borderRadius: 2, overflow: 'hidden', background: '#f0f0f0' }}>
              <div style={{ width: `${percent}%`, height: '100%', background: '#faad14' }} />
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

  const uniqueModels = Array.from(
    new Set(hourlyStats.flatMap((stat) => (stat.models ? Object.keys(stat.models) : [])))
  );
  const chartColors = ['#1890ff', '#52c41a', '#faad14', '#f5222d', '#722ed1', '#eb2f96', '#13c2c2', '#fa8c16'];

  return (
    <div className="dashboard-stats">
      {/* 今日运行概览与流量透视 */}
      <Card style={{ marginBottom: 24 }}>
        <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
          {/* 第一行：运行与总量概览 */}
          <div style={{ display: 'flex', alignItems: 'center', flexWrap: 'wrap', gap: 12 }}>
            <span style={{ fontWeight: 'bold', fontSize: 14, minWidth: 100 }}>
              <ThunderboltOutlined style={{ color: '#faad14', marginRight: 6 }} />
              今日运行概览:
            </span>
            <Tag color="blue" style={{ fontSize: 13, padding: '4px 10px' }}>
              <UserOutlined style={{ marginRight: 4 }} />
              活跃用户: <b>{summary.active_users}</b> {summary.total_users ? `/ ${summary.total_users} 人` : '人'}
            </Tag>
            <Tag color="green" style={{ fontSize: 13, padding: '4px 10px' }}>
              <CloudServerOutlined style={{ marginRight: 4 }} />
              最高并发: <b>{summary.peak_concurrency}</b>
            </Tag>
            <Tag color="gold" style={{ fontSize: 13, padding: '4px 10px' }}>
              <ThunderboltOutlined style={{ marginRight: 4 }} />
              总请求数: <b>{summary.today_requests.toLocaleString()}</b> 次
            </Tag>
            <Tag color="magenta" style={{ fontSize: 13, padding: '4px 10px' }}>
              <HistoryOutlined style={{ marginRight: 4 }} />
              Token 消耗: <b>{formatTokens(summary.today_tokens)}</b>
            </Tag>
          </div>

          {/* 第二行：会话与流量透视 */}
          <div style={{ display: 'flex', alignItems: 'center', flexWrap: 'wrap', gap: 12 }}>
            <span style={{ fontWeight: 'bold', fontSize: 14, minWidth: 100 }}>
              <CommentOutlined style={{ color: '#13c2c2', marginRight: 6 }} />
              会话流量透视:
            </span>
            <Tag color="cyan" style={{ fontSize: 13, padding: '4px 10px' }}>
              活跃会话: <b>{summary.today_sessions}</b> 个 {summary.today_sessions > 0 ? `(均深 ${summary.today_avg_session_depth.toFixed(1)} 次/会话)` : ''}
            </Tag>
            <Tag color="purple" style={{ fontSize: 13, padding: '4px 10px' }}>
              会话内请求: <b>{summary.today_session_requests.toLocaleString()}</b> 次 ({((summary.today_session_ratio || 0) * 100).toFixed(1)}%)
            </Tag>
            <Tag color="orange" style={{ fontSize: 13, padding: '4px 10px' }}>
              无会话独立调用: <b>{summary.today_no_session_requests.toLocaleString()}</b> 次 ({((1 - (summary.today_session_ratio || 0)) * 100).toFixed(1)}%)
            </Tag>
          </div>
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
