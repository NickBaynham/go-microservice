import { FormEvent, useEffect, useMemo, useState } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { resetPassword } from '../api'

export default function ResetPassword() {
  const [params] = useSearchParams()
  const nav = useNavigate()
  const tokenFromQuery = useMemo(() => params.get('token')?.trim() ?? '', [params])

  const [token, setToken] = useState(tokenFromQuery)
  const [password, setPassword] = useState('')
  const [err, setErr] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    const t = params.get('token')?.trim() ?? ''
    if (t) setToken(t)
  }, [params])

  async function onSubmit(e: FormEvent) {
    e.preventDefault()
    setErr(null)
    setBusy(true)
    try {
      await resetPassword(token, password)
      nav('/login', { replace: true, state: { resetOk: true } })
    } catch (x) {
      setErr(x instanceof Error ? x.message : 'reset failed')
    } finally {
      setBusy(false)
    }
  }

  return (
    <main>
      <h1>Set a new password</h1>
      <p>Paste the reset token from your email link, or open this page from the link directly.</p>
      <form onSubmit={onSubmit} data-testid="reset-form">
        {err && (
          <p role="alert" data-testid="reset-error">
            {err}
          </p>
        )}
        <label>
          Reset token
          <input
            name="token"
            type="text"
            value={token}
            onChange={(e) => setToken(e.target.value)}
            required
            autoComplete="off"
            data-testid="reset-token"
          />
        </label>
        <label>
          New password
          <input
            name="password"
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            required
            minLength={8}
            autoComplete="new-password"
            data-testid="reset-password"
          />
        </label>
        <button type="submit" disabled={busy} data-testid="reset-submit">
          {busy ? 'Updating…' : 'Update password'}
        </button>
      </form>
      <p>
        <Link to="/login">Back to login</Link>
      </p>
    </main>
  )
}
