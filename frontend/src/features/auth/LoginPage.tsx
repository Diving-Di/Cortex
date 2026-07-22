import { useNavigate } from 'react-router-dom'
import Auth from '../../components/auth/Auth'

export default function LoginPage() {
  const navigate = useNavigate()
  return <Auth onLogin={(token, username) => {
    localStorage.setItem('token', token)
    localStorage.setItem('username', username)
    navigate('/', { replace: true })
  }} />
}
