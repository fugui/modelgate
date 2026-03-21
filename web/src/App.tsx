import React, { useEffect, useState } from 'react';
import { BrowserRouter, Routes, Route, Navigate, useNavigate } from 'react-router-dom';
import { ConfigProvider, theme, Spin } from 'antd';
import zhCN from 'antd/locale/zh_CN';
import Login from './pages/Login';
import Register from './pages/Register';
import MainLayout from './components/MainLayout';
import Chat from './pages/Chat';
import UsageStats from './pages/UsageStats';
import DashboardStats from './pages/DashboardStats';
import APIKeyManage from './pages/APIKeyManage';
import Admin from './pages/Admin';
import BackendManage from './pages/BackendManage';
import api from './api';
import './App.css';

const App: React.FC = () => {
  return (
    <ConfigProvider
      locale={zhCN}
      theme={{
        algorithm: theme.defaultAlgorithm,
        token: {
          colorPrimary: '#1890ff',
        },
      }}
    >
      <BrowserRouter>
        <Routes>
          <Route path="/login" element={<Login />} />
          <Route path="/register" element={<Register />} />
          <Route
            path="/"
            element={
              <PrivateRoute>
                <MainLayout />
              </PrivateRoute>
            }
          >
            <Route index element={<Navigate to="/chat" replace />} />
            <Route path="chat" element={<Chat />} />
            <Route path="stats" element={<UsageStats />} />
            <Route path="keys" element={<APIKeyManage />} />
            <Route path="dashboard" element={<DashboardStats />} />
            <Route path="admin" element={<Navigate to="/admin/users" replace />} />
            <Route path="admin/models/:modelId/backends" element={<BackendManage />} />
            <Route path="admin/:tab" element={<Admin />} />
          </Route>
          {/* 默认路由 - 捕获所有未匹配的路径 */}
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </BrowserRouter>
    </ConfigProvider>
  );
};

const PrivateRoute: React.FC<{ children: React.ReactElement }> = ({ children }) => {
  const [isValidating, setIsValidating] = useState(true);
  const [isAuthenticated, setIsAuthenticated] = useState(false);
  const navigate = useNavigate();

  useEffect(() => {
    const validateToken = async () => {
      const token = localStorage.getItem('token');
      if (!token) {
        setIsAuthenticated(false);
        setIsValidating(false);
        return;
      }

      try {
        // 验证 token 是否有效
        await api.get('/api/v1/user/profile');
        setIsAuthenticated(true);
      } catch (err: any) {
        // Token 无效，清除登录状态
        if (err.response?.status === 401) {
          localStorage.removeItem('token');
          localStorage.removeItem('user');
        }
        setIsAuthenticated(false);
      } finally {
        setIsValidating(false);
      }
    };

    validateToken();
  }, [navigate]);

  if (isValidating) {
    return (
      <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '100vh' }}>
        <Spin size="large" tip="加载中..." />
      </div>
    );
  }

  if (!isAuthenticated) {
    return <Navigate to="/login" replace />;
  }

  return children;
};

export default App;
