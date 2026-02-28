import React, { useState, useRef, useEffect } from 'react';
import { Input, Button, Select, Card, message, Space, Tag, Empty, Spin } from 'antd';
import { SendOutlined, ClearOutlined, StopOutlined, KeyOutlined } from '@ant-design/icons';
import ReactMarkdown from 'react-markdown';
import { useNavigate } from 'react-router-dom';
import api from '../api';

const { TextArea } = Input;
const { Option } = Select;

interface Message {
  id: string;
  role: 'user' | 'assistant';
  content: string;
  timestamp: Date;
  model?: string;
}

interface Model {
  id: string;
  name: string;
}

const Chat: React.FC = () => {
  const [messages, setMessages] = useState<Message[]>([]);
  const [input, setInput] = useState('');
  const [selectedModel, setSelectedModel] = useState<string>('');
  const [models, setModels] = useState<Model[]>([]);
  const [loading, setLoading] = useState(false);
  const [hasApiKey, setHasApiKey] = useState(false);
  const abortControllerRef = useRef<AbortController | null>(null);
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const navigate = useNavigate();

  // 获取可用模型列表和 API Key 状态
  useEffect(() => {
    const fetchData = async () => {
      try {
        // 获取配额信息中的模型列表
        const quotaRes = await api.get('/api/v1/user/quota');
        const modelList = quotaRes.data.data?.models_allowed || [];
        setModels(modelList.map((m: string) => ({ id: m, name: m })));
        if (modelList.length > 0 && !selectedModel) {
          setSelectedModel(modelList[0]);
        }

        // 检查是否有 API Key
        const keysRes = await api.get('/api/v1/user/keys');
        const keys = keysRes.data.data || [];
        setHasApiKey(keys.length > 0);
      } catch (err) {
        console.error('Failed to fetch data:', err);
      }
    };
    fetchData();
  }, []);

  // 滚动到底部
  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages]);

  const handleSend = async () => {
    if (!input.trim() || !selectedModel) return;

    const userMessage: Message = {
      id: Date.now().toString(),
      role: 'user',
      content: input.trim(),
      timestamp: new Date(),
    };

    setMessages(prev => [...prev, userMessage]);
    setInput('');
    setLoading(true);

    // 创建 AbortController 用于取消请求
    abortControllerRef.current = new AbortController();

    try {
      // 调用聊天接口 - 使用代理端点，会自动使用当前用户的身份
      const response = await fetch('/v1/chat/completions', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          model: selectedModel,
          messages: [
            ...messages.map(m => ({ role: m.role, content: m.content })),
            { role: 'user', content: userMessage.content }
          ],
          stream: true,
        }),
        signal: abortControllerRef.current.signal,
      });

      if (!response.ok) {
        const errorData = await response.json().catch(() => ({}));
        throw new Error(errorData.error?.message || `请求失败: ${response.status}`);
      }

      // 处理流式响应
      const reader = response.body?.getReader();
      const decoder = new TextDecoder();
      let assistantContent = '';

      const assistantMessage: Message = {
        id: (Date.now() + 1).toString(),
        role: 'assistant',
        content: '',
        timestamp: new Date(),
        model: selectedModel,
      };

      setMessages(prev => [...prev, assistantMessage]);

      while (reader) {
        const { done, value } = await reader.read();
        if (done) break;

        const chunk = decoder.decode(value);
        const lines = chunk.split('\n');

        for (const line of lines) {
          if (line.startsWith('data: ')) {
            const data = line.slice(6);
            if (data === '[DONE]') continue;

            try {
              const parsed = JSON.parse(data);
              const content = parsed.choices?.[0]?.delta?.content || '';
              assistantContent += content;

              setMessages(prev =>
                prev.map(m =>
                  m.id === assistantMessage.id
                    ? { ...m, content: assistantContent }
                    : m
                )
              );
            } catch (e) {
              // 忽略解析错误
            }
          }
        }
      }
    } catch (err: any) {
      if (err.name === 'AbortError') {
        // 用户取消，不显示错误
        return;
      }
      message.error(err.message || '发送消息失败');
    } finally {
      setLoading(false);
      abortControllerRef.current = null;
    }
  };

  const handleStop = () => {
    if (abortControllerRef.current) {
      abortControllerRef.current.abort();
      setLoading(false);
    }
  };

  const handleClear = () => {
    setMessages([]);
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      handleSend();
    }
  };

  // 如果没有 API Key，显示引导
  if (!hasApiKey) {
    return (
      <Empty
        image={Empty.PRESENTED_IMAGE_SIMPLE}
        description={
          <Space direction="vertical" size="large" style={{ textAlign: 'center' }}>
            <div>
              <KeyOutlined style={{ fontSize: 48, color: '#1890ff' }} />
              <h3 style={{ marginTop: 16 }}>需要 API Key 才能使用聊天功能</h3>
              <p style={{ color: '#666' }}>请先创建一个 API Key，然后刷新页面</p>
            </div>
            <Button type="primary" onClick={() => navigate('/keys')}>
              去创建 API Key
            </Button>
          </Space>
        }
      />
    );
  }

  return (
    <div style={{ height: '100%', display: 'flex', flexDirection: 'column' }}>
      {/* 头部工具栏 */}
      <div style={{ 
        marginBottom: 16, 
        display: 'flex', 
        justifyContent: 'space-between', 
        alignItems: 'center',
        padding: '12px 16px',
        background: '#fff',
        borderRadius: 8,
        boxShadow: '0 1px 2px rgba(0,0,0,0.05)',
      }}>
        <Space>
          <span style={{ fontWeight: 500 }}>模型：</span>
          <Select
            value={selectedModel}
            onChange={setSelectedModel}
            style={{ width: 200 }}
            disabled={models.length === 0 || loading}
            placeholder="选择模型"
          >
            {models.map(model => (
              <Option key={model.id} value={model.id}>{model.name}</Option>
            ))}
          </Select>
          {models.length === 0 && (
            <Tag color="warning">暂无可用模型</Tag>
          )}
        </Space>

        <Button
          icon={<ClearOutlined />}
          onClick={handleClear}
          disabled={messages.length === 0 || loading}
        >
          清空对话
        </Button>
      </div>

      {/* 消息列表 */}
      <div style={{
        flex: 1,
        overflow: 'auto',
        border: '1px solid #f0f0f0',
        borderRadius: 8,
        padding: 16,
        marginBottom: 16,
        background: '#fafafa',
      }}>
        {messages.length === 0 ? (
          <div style={{
            height: '100%',
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'center',
            justifyContent: 'center',
            color: '#999',
          }}>
            <div style={{ fontSize: 48, marginBottom: 16 }}>💬</div>
            <div style={{ fontSize: 16, marginBottom: 8 }}>开始与 AI 助手对话</div>
            <div style={{ fontSize: 14 }}>选择模型后输入消息，按 Enter 发送</div>
          </div>
        ) : (
          messages.map(msg => (
            <div
              key={msg.id}
              style={{
                marginBottom: 16,
                display: 'flex',
                justifyContent: msg.role === 'user' ? 'flex-end' : 'flex-start',
              }}
            >
              <div
                style={{
                  maxWidth: '80%',
                  padding: 12,
                  borderRadius: 12,
                  background: msg.role === 'user' ? '#1890ff' : '#fff',
                  color: msg.role === 'user' ? '#fff' : 'inherit',
                  boxShadow: '0 1px 2px rgba(0,0,0,0.1)',
                }}
              >
                <div style={{ marginBottom: 8, display: 'flex', alignItems: 'center', gap: 8 }}>
                  <Tag color={msg.role === 'user' ? 'blue' : 'green'}>
                    {msg.role === 'user' ? '我' : 'AI'}
                  </Tag>
                  {msg.model && msg.role === 'assistant' && (
                    <Tag size="small" style={{ fontSize: 11 }}>
                      {msg.model}
                    </Tag>
                  )}
                  <span style={{ fontSize: 11, opacity: 0.6 }}>
                    {msg.timestamp.toLocaleTimeString()}
                  </span>
                </div>
                <div style={{ fontSize: 14, lineHeight: 1.7 }}>
                  {msg.role === 'assistant' ? (
                    <div style={{ color: '#333' }}>
                      <ReactMarkdown 
                        components={{
                          code: ({ children }: { children: React.ReactNode }) => (
                            <code style={{ 
                              background: '#f0f0f0', 
                              padding: '2px 6px', 
                              borderRadius: 4,
                              fontFamily: 'monospace',
                            }}>
                              {children}
                            </code>
                          ),
                          pre: ({ children }: { children: React.ReactNode }) => (
                            <pre style={{ 
                              background: '#f5f5f5', 
                              padding: 12, 
                              borderRadius: 8,
                              overflow: 'auto',
                              fontSize: 13,
                            }}>
                              {children}
                            </pre>
                          ),
                        }}
                      >
                        {msg.content || (loading && msg.id === messages[messages.length - 1]?.id ? (
                          <Spin size="small" />
                        ) : '')}
                      </ReactMarkdown>
                    </div>
                  ) : (
                    msg.content
                  )}
                </div>
              </div>
            </div>
          ))
        )}
        <div ref={messagesEndRef} />
      </div>

      {/* 输入框 */}
      <div style={{ 
        display: 'flex', 
        gap: 12,
        padding: '12px 16px',
        background: '#fff',
        borderRadius: 8,
        boxShadow: '0 -1px 2px rgba(0,0,0,0.05)',
      }}>
        <TextArea
          value={input}
          onChange={e => setInput(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder="输入消息，按 Enter 发送，Shift+Enter 换行..."
          autoSize={{ minRows: 2, maxRows: 6 }}
          style={{ flex: 1 }}
          disabled={loading || !selectedModel}
        />
        {loading ? (
          <Button
            danger
            icon={<StopOutlined />}
            onClick={handleStop}
            style={{ height: 'auto', minWidth: 80 }}
          >
            停止
          </Button>
        ) : (
          <Button
            type="primary"
            icon={<SendOutlined />}
            onClick={handleSend}
            disabled={!input.trim() || !selectedModel}
            style={{ height: 'auto', minWidth: 80 }}
          >
            发送
          </Button>
        )}
      </div>
    </div>
  );
};

export default Chat;
