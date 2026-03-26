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
  top_users_7d: {
    user_id: string;
    name: string;
    department: string;
    total_requests: number;
    total_tokens: number;
    daily_stats: {
      date: string;
      request_count: number;
      input_tokens: number;
      output_tokens: number;
    }[];
  }[];
}



const DashboardStats: React.FC = () => {
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<DashboardData | null>(null);

  const fetchStats = async () => {
    try {
      const [statsRes, hourlyRes, topUsersRes, topUsers7dRes] = await Promise.all([
        api.get('/api/v1/dashboard/stats'),
        api.get('/api/v1/dashboard/hourly'),
        api.get('/api/v1/dashboard/top-users'),
        api.get('/api/v1/dashboard/top-users-7d'),
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
        top_users_7d: topUsers7dRes.data.data || [],
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

  const { summary, hourly_stats: hourlyStats, top_users: topUsers, top_users_7d: topUsers7d } = data;

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

  // 生成最近7天的日期列表（使用本地时区，与后端保持一致）
  const last7Dates: string[] = [];
  for (let i = 6; i >= 0; i--) {
    const d = new Date();
    d.setDate(d.getDate() - i);
    const yyyy = d.getFullYear();
    const mm = String(d.getMonth() + 1).padStart(2, '0');
    const dd = String(d.getDate()).padStart(2, '0');
    last7Dates.push(`${yyyy}-${mm}-${dd}`);
  }

  const weekDayNames = ['周日', '周一', '周二', '周三', '周四', '周五', '周六'];
  const getWeekDayName = (dateStr: string) => {
    const d = new Date(dateStr + 'T00:00:00');
    return weekDayNames[d.getDay()];
  };
  const formatDateShort = (dateStr: string) => {
    const parts = dateStr.split('-');
    return `${parts[1]}-${parts[2]}`;
  };

  const findDailyStat = (dailyStats: any[], date: string) =>
    (dailyStats || []).find((s: any) => (s.date || '').slice(0, 10) === date);

  const topUser7dColumns = (() => {
    const dateColumns = last7Dates.map((date) => ({
      title: `${getWeekDayName(date)} ${formatDateShort(date)}`,
      key: `day_${date}`,
      width: 150,
      render: (_: any, record: any) => {
        const stat = findDailyStat(record.daily_stats, date);
        if (!stat || (stat.request_count === 0 && (stat.input_tokens || 0) === 0 && (stat.output_tokens || 0) === 0)) {
          return <span style={{ color: '#ccc' }}>-</span>;
        }
        return (
          <span>
            <span>{stat.request_count}次</span>
            <br />
            <span style={{ color: '#fa8c16', fontSize: 12 }}>↑{formatTokens(stat.input_tokens || 0)}</span>
            <span style={{ margin: '0 2px', fontSize: 12 }}>/</span>
            <span style={{ color: '#722ed1', fontSize: 12 }}>↓{formatTokens(stat.output_tokens || 0)}</span>
          </span>
        );
      },
    }));

    return [
      { title: '#', key: 'rank', width: 50, fixed: 'left' as const, render: (_: any, __: any, index: number) => index + 1 },
      { title: '用户名', dataIndex: 'name', key: 'name', width: 120, fixed: 'left' as const, render: (text: string, record: any) => text || record.user_id },
      ...dateColumns,
      {
        title: '总计',
        key: 'total',
        width: 150,
        fixed: 'right' as const,
        render: (_: any, record: any) => {
          const totalInput = (record.daily_stats || []).reduce((sum: number, s: any) => sum + (s.input_tokens || 0), 0);
          const totalOutput = (record.daily_stats || []).reduce((sum: number, s: any) => sum + (s.output_tokens || 0), 0);
          return (
            <span>
              <span>{record.total_requests}次</span>
              <br />
              <span style={{ color: '#fa8c16', fontSize: 12 }}>↑{formatTokens(totalInput)}</span>
              <span style={{ margin: '0 2px', fontSize: 12 }}>/</span>
              <span style={{ color: '#722ed1', fontSize: 12 }}>↓{formatTokens(totalOutput)}</span>
            </span>
          );
        },
        sorter: (a: any, b: any) => a.total_tokens - b.total_tokens,
      },
    ];
  })();

  const renderSummary7d = () => {
    const sumByDate = (date: string, field: string) =>
      topUsers7d.reduce((sum, u) => {
        const stat = findDailyStat(u.daily_stats, date);
        return sum + ((stat as any)?.[field] || 0);
      }, 0);
    const grandRequests = topUsers7d.reduce((sum, u) => sum + (u.total_requests || 0), 0);
    const grandInput = topUsers7d.reduce((sum, u) =>
      sum + (u.daily_stats || []).reduce((s: number, d: any) => s + (d.input_tokens || 0), 0), 0);
    const grandOutput = topUsers7d.reduce((sum, u) =>
      sum + (u.daily_stats || []).reduce((s: number, d: any) => s + (d.output_tokens || 0), 0), 0);

    return (
      <Table.Summary fixed>
        <Table.Summary.Row>
          <Table.Summary.Cell index={0} colSpan={2}>
            <strong>总计</strong>
          </Table.Summary.Cell>
          {last7Dates.map((date, idx) => {
            const req = sumByDate(date, 'request_count');
            const inp = sumByDate(date, 'input_tokens');
            const out = sumByDate(date, 'output_tokens');
            return (
              <Table.Summary.Cell key={date} index={idx + 2}>
                <span>
                  <strong>{req}次</strong>
                  <br />
                  <span style={{ color: '#fa8c16', fontSize: 12 }}>↑{formatTokens(inp)}</span>
                  <span style={{ margin: '0 2px', fontSize: 12 }}>/</span>
                  <span style={{ color: '#722ed1', fontSize: 12 }}>↓{formatTokens(out)}</span>
                </span>
              </Table.Summary.Cell>
            );
          })}
          <Table.Summary.Cell index={last7Dates.length + 2}>
            <span>
              <strong>{grandRequests}次</strong>
              <br />
              <span style={{ color: '#fa8c16', fontSize: 12 }}>↑{formatTokens(grandInput)}</span>
              <span style={{ margin: '0 2px', fontSize: 12 }}>/</span>
              <span style={{ color: '#722ed1', fontSize: 12 }}>↓{formatTokens(grandOutput)}</span>
            </span>
          </Table.Summary.Cell>
        </Table.Summary.Row>
      </Table.Summary>
    );
  };



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

      {/* 最近7天 TOP 20 用户 */}
      <Row gutter={16} style={{ marginBottom: 24 }}>
        <Col xs={24}>
          <Card title="最近7天 TOP 20 用户">
            {topUsers7d.length > 0 ? (
              <Table
                dataSource={topUsers7d}
                columns={topUser7dColumns}
                rowKey="user_id"
                pagination={false}
                size="small"
                scroll={{ x: 1500 }}
                summary={renderSummary7d}
              />
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
