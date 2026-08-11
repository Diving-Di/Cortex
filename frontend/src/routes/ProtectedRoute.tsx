import { useEffect, useState } from 'react';
import { Spin } from 'antd';
import { Navigate, Outlet, useLocation } from 'react-router-dom';
import { getSession, type SessionResponse } from '../api/auth';

export interface AuthenticatedOutletContext {
  session: SessionResponse;
  refreshSession: () => Promise<void>;
}

export default function ProtectedRoute() {
  const location = useLocation();
  const [session, setSession] = useState<SessionResponse | null>();

  async function refreshSession() {
    setSession(await getSession());
  }

  useEffect(() => {
    void refreshSession().catch(() => setSession(null));
    const handleUnauthorized = () => setSession(null);
    window.addEventListener('auth:unauthorized', handleUnauthorized);
    return () => window.removeEventListener('auth:unauthorized', handleUnauthorized);
  }, []);

  if (session === undefined) return <Spin fullscreen />;
  if (session === null) {
    return <Navigate to="/login" replace state={{ from: location.pathname }} />;
  }
  return <Outlet context={{ session, refreshSession }} />;
}
