import { FormEvent, useState } from 'react'
import { Link } from 'react-router-dom'
import { requestPasswordReset } from '../api'

export default function ForgotPassword() {
  const [email, setEmail] = useState('')
  const [done, setDone] = useState(false)
  const [err, setErr] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  async function onSubmit(e: FormEvent) {
    e.preventDefault()
    setErr(null)
    setBusy(true)
    try {
      await requestPasswordReset(email)
      setDone(true)
    } catch (x) {
      setErr(x instanceof Error ? x.message : 'request failed')
    } finally {
      setBusy(false)
    }
  }

  if (done) {
    return (
      <main>
        <h1>Check your email</h1>
        <p data-testid="forgot-done">
          If an account exists for that address, we sent password reset instructions.
        </p>
        <p>
          <Link to="/login">Back to login</Link>
        </p>
      </main>
    )
  }

  return (
    <main>
      <h1>Forgot password</h1>
      <p>Enter your email and we will send a reset link if you have an account.</p>
      <form onSubmit={onSubmit} data-testid="forgot-form">
        {err && (
          <p role="alert" data-testid="forgot-error">
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
            data-testid="forgot-email"
          />
        </label>
        <button type="submit" disabled={busy} data-testid="forgot-submit">
          {busy ? 'Sending…' : 'Send reset link'}
        </button>
      </form>
      <p>
        <Link to="/login">Back to login</Link>
      </p>
    </main>
  )
}
