import type { KeyboardEvent } from 'react'
import { useEffect, useRef, useState } from 'react'
import { Button, Input, Popconfirm, Spin, message as antdMessage } from 'antd'
import {
  DeleteOutlined,
  PlusOutlined,
  RobotOutlined,
  SendOutlined,
  UserOutlined,
} from '@ant-design/icons'
import type { ChatMessage, Conversation } from '../../types'
import {
  deleteConversation,
  fetchConversation,
  fetchConversations,
  sendMessage,
} from '../../api/chat'
import './Chat.css'

interface ChatProps {
  token: string
}

export default function Chat({ token }: ChatProps) {
  const [conversations, setConversations] = useState<Conversation[]>([])
  const [activeId, setActiveId] = useState<number | null>(null)
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [input, setInput] = useState('')
  const [sending, setSending] = useState(false)
  const [loadingMessages, setLoadingMessages] = useState(false)
  const listEndRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    loadConversations()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  useEffect(() => {
    listEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages, sending])

  async function loadConversations() {
    try {
      setConversations(await fetchConversations(token))
    } catch (e) {
      console.error(e)
    }
  }

  async function openConversation(id: number) {
    setActiveId(id)
    setLoadingMessages(true)
    try {
      const detail = await fetchConversation(token, id)
      setMessages(detail.messages)
    } catch (e) {
      antdMessage.error('加载对话失败')
    } finally {
      setLoadingMessages(false)
    }
  }

  function startNewConversation() {
    setActiveId(null)
    setMessages([])
  }

  async function handleSend() {
    const content = input.trim()
    if (!content || sending) return
    setInput('')
    setSending(true)

    const optimistic: ChatMessage = {
      id: Date.now(),
      role: 'user',
      content,
      created_at: new Date().toISOString(),
    }
    setMessages((prev) => [...prev, optimistic])

    try {
      const res = await sendMessage(token, content, activeId ?? undefined)
      setActiveId(res.conversation_id)
      setMessages((prev) => {
        const withoutOptimistic = prev.filter((m) => m.id !== optimistic.id)
        return [...withoutOptimistic, res.user_message, res.assistant_message]
      })
      await loadConversations()
    } catch (e) {
      antdMessage.error('发送失败，请重试')
      setMessages((prev) => prev.filter((m) => m.id !== optimistic.id))
      setInput(content)
    } finally {
      setSending(false)
    }
  }

  async function handleDelete(id: number) {
    try {
      await deleteConversation(token, id)
      if (id === activeId) startNewConversation()
      await loadConversations()
    } catch (e) {
      antdMessage.error('删除失败')
    }
  }

  function onKeyDown(e: KeyboardEvent<HTMLTextAreaElement>) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      handleSend()
    }
  }

  return (
    <div className="chat">
      <aside className="chat-sidebar">
        <Button type="primary" icon={<PlusOutlined />} block onClick={startNewConversation}>
          新建对话
        </Button>
        <div className="chat-conv-list">
          {conversations.map((conv) => (
            <div
              key={conv.id}
              className={`chat-conv-item${conv.id === activeId ? ' active' : ''}`}
              onClick={() => openConversation(conv.id)}
            >
              <span className="chat-conv-title">{conv.title}</span>
              <Popconfirm
                title="删除该对话？"
                onConfirm={(e) => {
                  e?.stopPropagation()
                  handleDelete(conv.id)
                }}
                onCancel={(e) => e?.stopPropagation()}
              >
                <DeleteOutlined
                  className="chat-conv-delete"
                  onClick={(e) => e.stopPropagation()}
                />
              </Popconfirm>
            </div>
          ))}
          {conversations.length === 0 ? <div className="chat-conv-empty">暂无历史对话</div> : null}
        </div>
      </aside>

      <section className="chat-main">
        <div className="chat-messages">
          {loadingMessages ? (
            <div className="chat-loading">
              <Spin />
            </div>
          ) : messages.length === 0 ? (
            <div className="chat-welcome">
              <RobotOutlined className="chat-welcome-icon" />
              <h2>开始和 AI 聊天吧</h2>
              <p>输入任意内容，开启一段多轮对话</p>
            </div>
          ) : (
            messages.map((msg) => (
              <div key={msg.id} className={`chat-bubble-row ${msg.role}`}>
                <div className="chat-avatar">
                  {msg.role === 'assistant' ? <RobotOutlined /> : <UserOutlined />}
                </div>
                <div className="chat-bubble">{msg.content}</div>
              </div>
            ))
          )}
          {sending ? (
            <div className="chat-bubble-row assistant">
              <div className="chat-avatar">
                <RobotOutlined />
              </div>
              <div className="chat-bubble">
                <Spin size="small" /> <span className="chat-typing">AI 正在思考…</span>
              </div>
            </div>
          ) : null}
          <div ref={listEndRef} />
        </div>

        <div className="chat-input-area">
          <Input.TextArea
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={onKeyDown}
            placeholder="输入消息，Enter 发送，Shift+Enter 换行"
            autoSize={{ minRows: 1, maxRows: 5 }}
          />
          <Button
            type="primary"
            icon={<SendOutlined />}
            loading={sending}
            onClick={handleSend}
          >
            发送
          </Button>
        </div>
      </section>
    </div>
  )
}
