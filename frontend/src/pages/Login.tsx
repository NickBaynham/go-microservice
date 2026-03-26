import { FormEvent, useState } from 'react'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import { loginUser } from '../api'
import { setToken } from '../auth'

export default function Login() {
  const nav = useNavigate()
  const loc = useLocation() as { state?: { registered?: boolean; resetOk?: boolean } }
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [err, setErr] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  async function onSubmit(e: FormEvent) {
    e.preventDefault()
    setErr(null)
    setBusy(true)
    try {
      const { token } = await loginUser(email, password)
      setToken(token)
      nav('/account', { replace: true })
    } catch (x) {
      setErr(x instanceof Error ? x.message : 'login failed')
    } finally {
      setBusy(false)
    }
  }

  return (
    <main>
      <h1>Log in</h1>
      {loc.state?.registered && (
        <p data-testid="login-flash">Account created — sign in below.</p>
      )}
      {loc.state?.resetOk && (
        <p data-testid="login-flash-reset">Password updated — sign in with your new password.</p>
      )}
      <form onSubmit={onSubmit} data-testid="login-form">
        {err && (
          <p role="alert" data-testid="login-error">
            {err}
          </p>
        )}
        <label>
          Email
          <input
            name="email"
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            required
            autoComplete="email"
            data-testid="login-email"
          />
        </label>
        <label>
          Password
          <input
            name="password"
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            required
            autoComplete="current-password"
            data-testid="login-password"
          />
        </label>
        <button type="submit" disabled={busy} data-testid="login-submit">
          {busy ? 'Signing in…' : 'Sign in'}
        </button>
      </form>
      <p>
        <Link to="/register">Create an account</Link>
        {' · '}
        <Link to="/forgot-password">Forgot password?</Link>
      </p>
    </main>
  )
}
