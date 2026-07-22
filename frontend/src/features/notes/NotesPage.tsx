import { Routes, Route } from 'react-router-dom'
import NoteList from './NoteList'
import NoteEditor from './NoteEditor'

export default function NotesPage({ token }: { token: string }) {
  return <Routes><Route index element={<NoteList token={token} />} /><Route path=":id" element={<NoteEditor token={token} />} /></Routes>
}
