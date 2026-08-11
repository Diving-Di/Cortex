import axios from 'axios';

export const http = axios.create({ withCredentials: true });

http.interceptors.response.use(undefined, (error) => {
  if (error.response?.status === 401) window.dispatchEvent(new Event('auth:unauthorized'));
  return Promise.reject(error);
});

export function authHeaders(_token: string) {
  return {};
}
