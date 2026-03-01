import React, { useState, useRef, useEffect } from 'react';
import { Input, Button, Select, message, Space, Tag, Spin } from 'antd';
import { SendOutlined, ClearOutlined, StopOutlined } from '@ant-design/icons';
import ReactMarkdown from 'react-markdown';
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
  const abortControllerRef = useRef<AbortController | null>(null);
  const messagesEndRef = useRef<HTMLDivElement>(null);
  // 使用 ref 存储当前流式输出的内容和消息 ID，避免闭包问题
  const streamingRef = useRef<{ id: string; content: string } | null>(null);

  // 获取可用模型列表
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
      // 调用聊天接口 - 使用 JWT Token 认证
      const token = localStorage.getItem('token');
      const response = await fetch('/v1/chat/completions', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${token}`,
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

      const assistantMessageId = (Date.now() + 1).toString();
      const assistantMessage: Message = {
        id: assistantMessageId,
        role: 'assistant',
        content: '',
        timestamp: new Date(),
        model: selectedModel,
      };

      // 初始化 streamingRef
      streamingRef.current = { id: assistantMessageId, content: '' };

      setMessages(prev => [...prev, assistantMessage]);

      // 用于累积不完整的 SSE 行
      let buffer = '';

      // 批量更新：每 50ms 更新一次 UI
      let pendingUpdate = false;
      const scheduleUpdate = () => {
        if (pendingUpdate) return;
        pendingUpdate = true;
        setTimeout(() => {
          pendingUpdate = false;
          const current = streamingRef.current;
          if (current) {
            setMessages(prev =>
              prev.map(m =>
                m.id === current.id
                  ? { ...m, content: current.content }
                  : m
              )
            );
          }
        }, 50);
      };

      while (reader) {
        const { done, value } = await reader.read();
        if (done) break;

        const chunk = decoder.decode(value);

        // 累积到 buffer
        buffer += chunk;

        // 处理完整的行 (SSE 格式: data: {...}\n\n)
        let lineEnd;
        while ((lineEnd = buffer.indexOf('\n')) !== -1) {
          const line = buffer.substring(0, lineEnd);
          buffer = buffer.substring(lineEnd + 1);

          const trimmedLine = line.trim();
          if (!trimmedLine) continue;

          if (trimmedLine.startsWith('data:')) {
            const data = trimmedLine.slice(5).trim();
            if (data === '[DONE]') continue;

            try {
              const parsed = JSON.parse(data);
              // 只使用 delta.content，忽略 reasoning_content（非 Thinking 模式）
              const delta = parsed.choices?.[0]?.delta || {};
              const text = delta.content || '';

              if (text && streamingRef.current) {
                streamingRef.current.content += text;
                scheduleUpdate();
                // 直接更新 DOM 绕过 React
                const contentEl = document.getElementById('streaming-content-' + assistantMessageId);
                if (contentEl) {
                  contentEl.textContent = streamingRef.current.content;
                }
              }
            } catch {
              // 忽略解析错误
            }
          }
        }
      }

      // 确保最后的内容被更新
      const final = streamingRef.current;
      if (final) {
        setMessages(prev =>
          prev.map(m =>
            m.id === final.id
              ? { ...m, content: final.content }
              : m
          )
        );
      }

      // 清理 ref
      streamingRef.current = null;
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
                    <span style={{ fontSize: 11, padding: '0 4px', background: '#f0f0f0', borderRadius: 4 }}>
                      {msg.model}
                    </span>
                  )}
                  <span style={{ fontSize: 11, opacity: 0.6 }}>
                    {msg.timestamp.toLocaleTimeString()}
                  </span>
                </div>
                <div style={{ fontSize: 14, lineHeight: 1.7 }}>
                  {msg.role === 'assistant' ? (
                    <div style={{ color: '#333' }}>
                      {/* 流式内容显示区域 - id 用于直接 DOM 更新 */}
                      {loading && msg.id === messages[messages.length - 1]?.id ? (
                        // 流式输出中：显示纯文本
                        <div id={'streaming-content-' + msg.id} style={{whiteSpace: 'pre-wrap', minHeight: 20}}>
                          {msg.content || ''}
                        </div>
                      ) : (
                        // 流式完成后：使用 Markdown 渲染
                        <div className="markdown-content">
                          <ReactMarkdown
                            components={{
                              code: ({ node, inline, className, children, ...props }: any) => (
                                inline ? (
                                  <code style={{
                                    background: '#f0f0f0',
                                    padding: '2px 6px',
                                    borderRadius: 4,
                                    fontFamily: 'monospace',
                                    fontSize: '0.9em',
                                  }} {...props}>
                                    {children}
                                  </code>
                                ) : (
                                  <pre style={{
                                    background: '#f5f5f5',
                                    padding: 12,
                                    borderRadius: 8,
                                    overflow: 'auto',
                                    fontSize: 13,
                                    border: '1px solid #e8e8e8',
                                  }}>
                                    <code style={{ fontFamily: 'monospace' }} {...props}>
                                      {children}
                                    </code>
                                  </pre>
                                )
                              ),
                              p: ({ children }: any) => (
                                <p style={{ margin: '0.5em 0' }}>{children}</p>
                              ),
                              ul: ({ children }: any) => (
                                <ul style={{ paddingLeft: 20, margin: '0.5em 0' }}>{children}</ul>
                              ),
                              ol: ({ children }: any) => (
                                <ol style={{ paddingLeft: 20, margin: '0.5em 0' }}>{children}</ol>
                              ),
                              li: ({ children }: any) => (
                                <li style={{ margin: '0.25em 0' }}>{children}</li>
                              ),
                              h1: ({ children }: any) => (
                                <h1 style={{ fontSize: 20, margin: '0.5em 0', fontWeight: 600 }}>{children}</h1>
                              ),
                              h2: ({ children }: any) => (
                                <h2 style={{ fontSize: 18, margin: '0.5em 0', fontWeight: 600 }}>{children}</h2>
                              ),
                              h3: ({ children }: any) => (
                                <h3 style={{ fontSize: 16, margin: '0.5em 0', fontWeight: 600 }}>{children}</h3>
                              ),
                              blockquote: ({ children }: any) => (
                                <blockquote style={{
                                  borderLeft: '4px solid #ddd',
                                  paddingLeft: 12,
                                  margin: '0.5em 0',
                                  color: '#666',
                                }}>{children}</blockquote>
                              ),
                              a: ({ href, children }: any) => (
                                <a href={href} target="_blank" rel="noopener noreferrer" style={{ color: '#1890ff' }}>
                                  {children}
                                </a>
                              ),
                              table: ({ children }: any) => (
                                <table style={{
                                  borderCollapse: 'collapse',
                                  width: '100%',
                                  margin: '0.5em 0',
                                }}>{children}</table>
                              ),
                              th: ({ children }: any) => (
                                <th style={{
                                  border: '1px solid #ddd',
                                  padding: '8px 12px',
                                  background: '#f5f5f5',
                                  fontWeight: 600,
                                }}>{children}</th>
                              ),
                              td: ({ children }: any) => (
                                <td style={{
                                  border: '1px solid #ddd',
                                  padding: '8px 12px',
                                }}>{children}</td>
                              ),
                            }}
                          >
                            {msg.content || ''}
                          </ReactMarkdown>
                        </div>
                      )}
                      {loading && msg.id === messages[messages.length - 1]?.id && (
                        <Spin size="small" style={{ marginLeft: 8 }} />
                      )}
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
