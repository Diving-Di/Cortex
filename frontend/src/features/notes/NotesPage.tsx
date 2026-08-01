import { Navigate, Routes, Route } from 'react-router-dom';
import NoteList from './NoteList';
import NoteEditor from './NoteEditor';
import TemplatesPage from '../templates/TemplatesPage';

export default function NotesPage({ token }: { token: string }) {
  const preferred = localStorage.getItem('diary:notes-section');
  return (
    <Routes>
      <Route
        index
        element={
          preferred === 'list' ? <Navigate to="list" replace /> : <TemplatesPage token={token} />
        }
      />
      <Route path="list" element={<NoteList token={token} />} />
      <Route path=":id" element={<NoteEditor token={token} />} />
    </Routes>
  );
}
