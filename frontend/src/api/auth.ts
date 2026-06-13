import { API_BASE_URL } from './config'
import { http } from './http'

interface LoginPayload {
  username: string
  password: string
}

interface LoginResponse {
  token: string
}

interface RegisterPayload extends LoginPayload {
  email: string
}

export async function loginUser(payload: LoginPayload): Promise<LoginResponse> {
  const res = await http.post<LoginResponse>(`${API_BASE_URL}/login/`, payload)
  return res.data
}

export async function registerUser(payload: RegisterPayload): Promise<void> {
  await http.post(`${API_BASE_URL}/register/`, payload)
}
