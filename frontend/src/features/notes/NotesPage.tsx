import { Navigate, Routes, Route } from 'react-router-dom';
import NoteList from './NoteList';
import NoteEditor from './NoteEditor';
import TemplatesPage from '../templates/TemplatesPage';

export default function NotesPage() {
  return (
    <Routes>
      <Route index element={<Navigate to="list" replace />} />
      <Route path="list" element={<NoteList />} />
      <Route path="templates" element={<TemplatesPage />} />
      <Route path=":id" element={<NoteEditor />} />
    </Routes>
  );
}
