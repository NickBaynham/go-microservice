import { FormEvent, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { registerUser } from '../api'
import TurnstileField from '../components/TurnstileField'
import { turnstileSiteKey } from '../envPublic'

export default function Register() {
  const nav = useNavigate()
  const [name, setName] = useState('')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [turnstileToken, setTurnstileToken] = useState('')
  const [err, setErr] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  async function onSubmit(e: FormEvent) {
    e.preventDefault()
    setErr(null)
    if (turnstileSiteKey() && !turnstileToken) {
      setErr('Complete the verification challenge.')
      return
    }
    setBusy(true)
    try {
      const { user } = await registerUser({
        name,
        email,
        password,
        turnstile_token: turnstileToken || undefined,
      })
      const needsVerification = user.email_verified === false
      nav('/login', { state: { registered: true, needsVerification } })
    } catch (x) {
      setErr(x instanceof Error ? x.message : 'register failed')
    } finally {
      setBusy(false)
    }
  }

  return (
    <main>
      <h1>Sign up</h1>
      <form onSubmit={onSubmit} data-testid="register-form">
        {err && (
          <p role="alert" data-testid="register-error">
            {err}
          </p>
        )}
        <label>
          Name
          <input
            name="name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            required
            minLength={2}
            autoComplete="name"
            data-testid="register-name"
          />
        </label>
        <label>
          Email
          <input
            name="email"
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            required
            autoComplete="email"
            data-testid="register-email"
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
            minLength={8}
            autoComplete="new-password"
            data-testid="register-password"
          />
        </label>
        <TurnstileField onToken={setTurnstileToken} />
        <button type="submit" disabled={busy} data-testid="register-submit">
          {busy ? 'Creating…' : 'Create account'}
        </button>
      </form>
      <p>
        <Link to="/login">Already have an account? Log in</Link>
      </p>
    </main>
  )
}
