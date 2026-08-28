import React, { useEffect, useState } from 'react';
import { Card, Tag, Row, Col, Progress } from 'antd';
import {
  ThunderboltOutlined,
  CommentOutlined,
  ApartmentOutlined,
  FieldTimeOutlined,
  AppstoreOutlined,
} from '@ant-design/icons';
import { BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer } from 'recharts';
import api from '../api';
import { MetricCard } from '../components/dashboard/MetricCard';
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

      {/* 顶部个人配额与会话运行指标卡片 */}
      <Row gutter={[16, 16]} style={{ marginBottom: 24, display: 'flex', flexWrap: 'wrap' }}>
        <Col xs={24} sm={12} md={8} lg={8} style={{ flex: '1 1 200px' }}>
          <MetricCard
            title="今日请求 / 限额"
            value={quota.daily_requests_used?.toLocaleString() || 0}
            suffix={quota.daily_requests_limit ? `/ ${quota.daily_requests_limit.toLocaleString()} 次` : '次'}
            icon={<ThunderboltOutlined />}
            iconColor="#1890ff"
            iconBg="#e6f7ff"
            footer={
              <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                <div style={{ display: 'flex', justifyContent: 'space-between' }}>
                  <span>已用配额</span>
                  <b style={{ color: requestUsagePercent > 90 ? '#f5222d' : '#1890ff' }}>{requestUsagePercent}%</b>
                </div>
                <Progress percent={requestUsagePercent} size="small" showInfo={false} status={requestUsagePercent > 90 ? 'exception' : 'active'} />
              </div>
            }
          />
        </Col>
        <Col xs={24} sm={12} md={8} lg={8} style={{ flex: '1 1 200px' }}>
          <MetricCard
            title="今日活跃会话"
            value={quota.daily_sessions_count || 0}
            suffix="个"
            icon={<CommentOutlined />}
            iconColor="#13c2c2"
            iconBg="#e6fffb"
            footer={
              <div style={{ display: 'flex', justifyContent: 'space-between' }}>
                <span>会话内请求</span>
                <b style={{ color: '#13c2c2' }}>{quota.daily_session_requests || 0} 次 ({((quota.session_request_ratio || 0) * 100).toFixed(0)}%)</b>
              </div>
            }
          />
        </Col>
        <Col xs={24} sm={12} md={8} lg={8} style={{ flex: '1 1 200px' }}>
          <MetricCard
            title="平均会话深度"
            value={(quota.avg_session_depth || 0).toFixed(1)}
            suffix="次/会话"
            icon={<ApartmentOutlined />}
            iconColor="#722ed1"
            iconBg="#f9f0ff"
            footer={
              <div style={{ display: 'flex', justifyContent: 'space-between' }}>
                <span>无会话独立调用</span>
                <b style={{ color: '#fa8c16' }}>{quota.daily_no_session_requests || 0} 次</b>
              </div>
            }
          />
        </Col>
        <Col xs={24} sm={12} md={8} lg={8} style={{ flex: '1 1 200px' }}>
          <MetricCard
            title="速率限制与重置"
            value={quota.rate_limit || 0}
            suffix={`次/${quota.rate_window || 60}s`}
            icon={<FieldTimeOutlined />}
            iconColor="#52c41a"
            iconBg="#f6ffed"
            footer={
              <div style={{ display: 'flex', justifyContent: 'space-between' }}>
                <span>重置时间</span>
                <b style={{ color: '#555' }}>{quota.reset_time || '每日 00:00'}</b>
              </div>
            }
          />
        </Col>
        <Col xs={24} sm={12} md={8} lg={8} style={{ flex: '1 1 200px' }}>
          <MetricCard
            title="授权可用模型"
            value={quota.models_allowed?.length || 0}
            suffix="个模型"
            icon={<AppstoreOutlined />}
            iconColor="#faad14"
            iconBg="#fffbe6"
            footer={
              <div style={{ display: 'flex', overflowX: 'auto', gap: 4, whiteSpace: 'nowrap', paddingBottom: 2 }}>
                {quota.models_allowed && quota.models_allowed.length > 0 ? (
                  quota.models_allowed.map((model: string) => (
                    <Tag key={model} style={{ margin: 0, fontSize: 11, padding: '0 4px' }}>
                      {model}
                    </Tag>
                  ))
                ) : (
                  <span style={{ color: '#aaa' }}>全模型可用</span>
                )}
              </div>
            }
          />
        </Col>
      </Row>

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
