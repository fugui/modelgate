import React from 'react';
import { Card } from 'antd';

export interface MetricCardProps {
  title: string;
  value: number | string;
  suffix?: React.ReactNode;
  icon?: React.ReactNode;
  iconColor?: string;
  iconBg?: string;
  valueStyle?: React.CSSProperties;
  footer?: React.ReactNode;
  onClick?: () => void;
}

export const MetricCard: React.FC<MetricCardProps> = ({
  title,
  value,
  suffix,
  icon,
  iconColor = '#1890ff',
  iconBg = 'rgba(24, 144, 255, 0.08)',
  valueStyle,
  footer,
  onClick,
}) => {
  return (
    <Card
      bordered={false}
      hoverable
      style={{
        borderRadius: 8,
        height: '100%',
        boxShadow: '0 1px 4px rgba(0, 21, 41, 0.06)',
      }}
      bodyStyle={{
        padding: '16px 18px',
        display: 'flex',
        flexDirection: 'column',
        height: '100%',
        justifyContent: 'space-between',
      }}
      onClick={onClick}
    >
      <div>
        {/* Header: Title and Icon Badge */}
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 8 }}>
          <span style={{ fontSize: 13, color: '#666', fontWeight: 500 }}>{title}</span>
          {icon && (
            <div
              style={{
                width: 32,
                height: 32,
                borderRadius: 6,
                backgroundColor: iconBg,
                color: iconColor,
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                fontSize: 16,
              }}
            >
              {icon}
            </div>
          )}
        </div>

        {/* Value */}
        <div style={{ display: 'flex', alignItems: 'baseline', marginBottom: footer ? 8 : 0 }}>
          <span
            style={{
              fontSize: 24,
              fontWeight: 700,
              color: '#1f1f1f',
              lineHeight: 1.2,
              letterSpacing: '-0.3px',
              ...valueStyle,
            }}
          >
            {value}
          </span>
          {suffix && (
            <span style={{ fontSize: 12, color: '#888', marginLeft: 6, fontWeight: 'normal' }}>
              {suffix}
            </span>
          )}
        </div>
      </div>

      {/* Footer */}
      {footer && (
        <div
          style={{
            paddingTop: 8,
            borderTop: '1px solid #f0f0f0',
            fontSize: 12,
            color: '#8c8c8c',
            marginTop: 4,
          }}
        >
          {footer}
        </div>
      )}
    </Card>
  );
};

export default MetricCard;

