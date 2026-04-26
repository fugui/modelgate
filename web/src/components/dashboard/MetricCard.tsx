import React from 'react';
import { Card, Statistic } from 'antd';

interface MetricCardProps {
  title: string;
  value: number | string;
  prefix?: React.ReactNode;
  suffix?: React.ReactNode;
  valueStyle?: React.CSSProperties;
}

export const MetricCard: React.FC<MetricCardProps> = ({ title, value, prefix, suffix, valueStyle }) => {
  return (
    <Card bordered={false} hoverable>
      <Statistic
        title={title}
        value={value}
        prefix={prefix}
        suffix={suffix}
        valueStyle={{ color: '#1890ff', fontWeight: 'bold', ...valueStyle }}
      />
    </Card>
  );
};

export default MetricCard;
