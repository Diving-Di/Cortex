import Diary from '../../components/diary/Diary'

export default function NotesPage({ token }: { token: string }) {
  return <Diary token={token} />
}
