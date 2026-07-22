import Chat from '../../components/chat/Chat'

export default function DashboardPage({ token }: { token: string }) {
  return <Chat token={token} />
}
