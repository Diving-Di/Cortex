import type { ChatResponse, Conversation, ConversationDetail } from '../types'
import { authHeaders, http } from './http'
import { apiUrls } from './urls'

export async function fetchConversations(token: string): Promise<Conversation[]> {
  const res = await http.get<Conversation[]>(apiUrls.chat.conversations, {
    headers: authHeaders(token),
  })
  return res.data || []
}

export async function fetchConversation(token: string, id: number): Promise<ConversationDetail> {
  const res = await http.get<ConversationDetail>(apiUrls.chat.conversation(id), {
    headers: authHeaders(token),
  })
  return res.data
}

export async function sendMessage(
  token: string,
  content: string,
  conversationId?: number,
): Promise<ChatResponse> {
  const res = await http.post<ChatResponse>(
    apiUrls.chat.send,
    { content, conversation_id: conversationId ?? null },
    { headers: authHeaders(token) },
  )
  return res.data
}

export async function deleteConversation(token: string, id: number): Promise<void> {
  await http.delete(apiUrls.chat.conversation(id), { headers: authHeaders(token) })
}
