import React, { useEffect, useState, useMemo } from 'react';
import { Button } from 'antd';
import { SyncOutlined } from '@ant-design/icons';
import api from '../api';
import AccessLogsTable from '../components/AccessLogsTable';

const AdminLogs: React.FC = () => {
  const [accessLogs, setAccessLogs] = useState<any[]>([]);
  const [users, setUsers] = useState<any[]>([]);
  const [loading, setLoading] = useState(false);

  const fetchData = async () => {
    setLoading(true);
    try {
      const [logsRes, usersRes] = await Promise.all([
        api.get('/api/v1/admin/access-logs?detailed=true&limit=50'),
        api.get('/api/v1/admin/users?page_size=1000') // Fetch a large enough page to get all users roughly
      ]);
      setAccessLogs(logsRes.data.data || []);
      setUsers(usersRes.data.data || []);
    } catch (err) {
      console.error('Failed to fetch admin access logs:', err);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchData();
  }, []);

  const userMap = useMemo(() => {
    const map: Record<string, string> = {};
    users.forEach(u => {
      map[u.id] = u.name || u.email || u.id;
    });
    return map;
  }, [users]);

  return (
    <>
      <div style={{ marginBottom: 16, display: 'flex', justifyContent: 'flex-end' }}>
        <Button icon={<SyncOutlined />} onClick={fetchData} loading={loading}>
          刷新
        </Button>
      </div>
      <AccessLogsTable 
        logs={accessLogs} 
        loading={loading} 
        isAdmin={true} 
        userMap={userMap} 
        scroll={{ y: 600 }} 
      />
    </>
  );
};

export default AdminLogs;
