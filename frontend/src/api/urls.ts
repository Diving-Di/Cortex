import { API_BASE_URL } from './config'

export const apiUrls = {
  auth: {
    login: `${API_BASE_URL}/login/`,
    register: `${API_BASE_URL}/register/`,
  },
  chat: {
    send: `${API_BASE_URL}/chat/`,
    conversations: `${API_BASE_URL}/chat/conversations/`,
    conversation: (id: number) => `${API_BASE_URL}/chat/conversations/${id}/`,
  },
  diary: {
    list: `${API_BASE_URL}/diary/`,
    create: `${API_BASE_URL}/diary/`,
    detail: (id: number) => `${API_BASE_URL}/diary/${id}/`,
  },
}
