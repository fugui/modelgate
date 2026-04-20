import React, { useState, useEffect } from 'react';
import { Card, Table, Empty, message } from 'antd';
import api from '../api';

interface TopUser7d {
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
}

const AdminTopUsers: React.FC = () => {
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<TopUser7d[]>([]);

  const fetchStats = async () => {
    try {
      setLoading(true);
      const res = await api.get('/api/v1/dashboard/top-users-7d');
      setData(res.data.data || []);
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

  const formatTokens = (num: number) => {
    if (num >= 1000000) {
      return (num / 1000000).toFixed(2) + 'M';
    }
    if (num >= 1000) {
      return (num / 1000).toFixed(1) + 'K';
    }
    return num.toString();
  };

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
      data.reduce((sum, u) => {
        const stat = findDailyStat(u.daily_stats, date);
        return sum + ((stat as any)?.[field] || 0);
      }, 0);
    const grandRequests = data.reduce((sum, u) => sum + (u.total_requests || 0), 0);
    const grandInput = data.reduce((sum, u) =>
      sum + (u.daily_stats || []).reduce((s: number, d: any) => s + (d.input_tokens || 0), 0), 0);
    const grandOutput = data.reduce((sum, u) =>
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
    <Card title="最近7天 TOP 20 用户">
      {data.length > 0 ? (
        <Table
          dataSource={data}
          columns={topUser7dColumns}
          rowKey="user_id"
          pagination={false}
          size="small"
          scroll={{ x: 1500 }}
          loading={loading}
          summary={renderSummary7d}
        />
      ) : (
        <Empty description={loading ? '加载中...' : '暂无数据'} style={{ padding: '60px 0' }} />
      )}
    </Card>
  );
};

export default AdminTopUsers;
