import { API_BASE_URL } from './config';

export const apiUrls = {
  auth: {
    login: `${API_BASE_URL}/v1/auth/login`,
    register: `${API_BASE_URL}/v1/auth/register`,
    logout: `${API_BASE_URL}/v1/auth/logout`,
    session: `${API_BASE_URL}/v1/auth/session`,
  },
};
