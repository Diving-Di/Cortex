import { http } from './http'
import { apiUrls } from './urls'

interface LoginPayload {
  username: string
  password: string
}

interface LoginResponse {
  token: string
  username: string
}

interface RegisterPayload extends LoginPayload {
  email: string
}

export async function loginUser(payload: LoginPayload): Promise<LoginResponse> {
  const res = await http.post<LoginResponse>(apiUrls.auth.login, payload)
  return res.data
}

export async function registerUser(payload: RegisterPayload): Promise<void> {
  await http.post(apiUrls.auth.register, payload)
}
