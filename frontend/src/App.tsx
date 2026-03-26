import { Link, Navigate, Route, Routes } from 'react-router-dom'
import './App.css'
import Account from './pages/Account'
import ForgotPassword from './pages/ForgotPassword'
import Login from './pages/Login'
import Register from './pages/Register'
import ResetPassword from './pages/ResetPassword'
import VerifyEmail from './pages/VerifyEmail'

export default function App() {
  return (
    <div className="app">
      <header className="app-header">
        <h1 className="app-title">User service</h1>
        <nav>
          <Link to="/login">Login</Link>
          {' · '}
          <Link to="/register">Register</Link>
          {' · '}
          <Link to="/forgot-password">Forgot password</Link>
          {' · '}
          <Link to="/account">Account</Link>
        </nav>
      </header>
      <Routes>
        <Route path="/" element={<Navigate to="/login" replace />} />
        <Route path="/register" element={<Register />} />
        <Route path="/login" element={<Login />} />
        <Route path="/forgot-password" element={<ForgotPassword />} />
        <Route path="/reset-password" element={<ResetPassword />} />
        <Route path="/verify-email" element={<VerifyEmail />} />
        <Route path="/account" element={<Account />} />
        <Route path="*" element={<Navigate to="/login" replace />} />
      </Routes>
    </div>
  )
}
