import { Outlet } from 'react-router-dom'
import { Header } from './Header'

export function AuthedLayout() {
  return (
    <>
      <Header />
      <Outlet />
    </>
  )
}
