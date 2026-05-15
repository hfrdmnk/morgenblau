import { BrowserRouter, Route, Routes } from 'react-router-dom'
import { AuthedLayout } from './components/AuthedLayout'
import { Home } from './pages/Home'
import { Login } from './pages/Login'
import { Sources } from './pages/Sources'

export default function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/login" element={<Login />} />
        <Route element={<AuthedLayout />}>
          <Route path="/" element={<Home />} />
          <Route path="/sources" element={<Sources />} />
          <Route path="*" element={<Home />} />
        </Route>
      </Routes>
    </BrowserRouter>
  )
}
