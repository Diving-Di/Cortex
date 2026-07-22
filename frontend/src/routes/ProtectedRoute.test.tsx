import { cleanup, render, screen } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { afterEach, expect, test } from 'vitest'
import ProtectedRoute from './ProtectedRoute'

afterEach(() => cleanup())

function renderRoutes() {
  return render(<MemoryRouter initialEntries={['/private']}><Routes>
    <Route path="/login" element={<div>login page</div>} />
    <Route element={<ProtectedRoute />}><Route path="/private" element={<div>private page</div>} /></Route>
  </Routes></MemoryRouter>)
}

test('redirects anonymous users to login', () => {
  localStorage.clear()
  renderRoutes()
  expect(screen.getByText('login page')).toBeInTheDocument()
})

test('renders protected content for authenticated users', () => {
  localStorage.setItem('token', 'test-token')
  renderRoutes()
  expect(screen.getByText('private page')).toBeInTheDocument()
})
