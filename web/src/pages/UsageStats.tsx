import React, { useEffect, useState } from 'react';
import { Card, Descriptions, Tag, Statistic, Row, Col, Progress } from 'antd';
import { BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer } from 'recharts';
import api from '../api';
import AccessLogsTable from '../components/AccessLogsTable';

const UsageStats: React.FC = () => {
  const [quota, setQuota] = useState<any>({});
  const [_usageRecords, setUsageRecords] = useState<any[]>([]);
  const [weeklyData, setWeeklyData] = useState<any[]>([]);
  const [accessLogs, setAccessLogs] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);

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
        <AccessLogsTable 
          logs={accessLogs} 
          loading={loading} 
          isAdmin={false} 
          scroll={{ y: 400 }} 
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
    </div>
  );
};

export default UsageStats;
