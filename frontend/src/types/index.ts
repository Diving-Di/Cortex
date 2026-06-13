export interface ChatMessage {
  id: number
  role: 'user' | 'assistant'
  content: string
  created_at: string
}

export interface Conversation {
  id: number
  title: string
  created_at: string
  updated_at: string
}

export interface ConversationDetail extends Conversation {
  messages: ChatMessage[]
}

export interface ChatResponse {
  conversation_id: number
  title: string
  user_message: ChatMessage
  assistant_message: ChatMessage
}

export interface DiaryEntry {
  id: number
  image: string | null
  content: string
  created_at: string
}
