import { Navigate, Routes, Route } from 'react-router-dom';
import NoteList from './NoteList';
import NoteEditor from './NoteEditor';
import TemplatesPage from '../templates/TemplatesPage';

export default function NotesPage() {
  const preferred = localStorage.getItem('cortex:notes-section');
  return (
    <Routes>
      <Route
        index
        element={preferred === 'list' ? <Navigate to="list" replace /> : <TemplatesPage />}
      />
      <Route path="list" element={<NoteList />} />
      <Route path=":id" element={<NoteEditor />} />
    </Routes>
  );
}
