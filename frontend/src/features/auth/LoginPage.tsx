import { useNavigate } from 'react-router-dom';
import Auth from '../../components/auth/Auth';

export default function LoginPage() {
  const navigate = useNavigate();
  return (
    <Auth
      onLogin={() => {
        navigate('/', { replace: true });
      }}
    />
  );
}
