import React from 'react';
import { Card, Table, Typography } from 'antd';
import type { ColumnsType } from 'antd/es/table';

const { Title } = Typography;

interface TopListProps {
  title: React.ReactNode;
  dataSource: any[];
  columns: ColumnsType<any>;
  rowKey?: string;
  extra?: React.ReactNode;
  scroll?: any;
  loading?: boolean;
  summary?: (data: readonly any[]) => React.ReactNode;
}

export const TopList: React.FC<TopListProps> = ({ title, dataSource, columns, rowKey = 'id', extra, scroll, loading, summary }) => {
  return (
    <Card
      title={<Title level={5} style={{ margin: 0 }}>{title}</Title>}
      bordered={false}
      extra={extra}
    >
      <Table
        dataSource={dataSource}
        columns={columns}
        rowKey={rowKey}
        pagination={false}
        size="small"
        scroll={scroll}
        loading={loading}
        summary={summary}
      />
    </Card>
  );
};

export default TopList;
