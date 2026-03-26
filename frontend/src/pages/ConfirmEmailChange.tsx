import { FormEvent, useEffect, useMemo, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { confirmEmailChange } from '../api'

export default function ConfirmEmailChange() {
  const [params] = useSearchParams()
  const tokenFromQuery = useMemo(() => params.get('token')?.trim() ?? '', [params])

  const [token, setToken] = useState(tokenFromQuery)
  const [err, setErr] = useState<string | null>(null)
  const [ok, setOk] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    const t = params.get('token')?.trim() ?? ''
    if (t) setToken(t)
  }, [params])

  async function onSubmit(e: FormEvent) {
    e.preventDefault()
    setErr(null)
    setOk(null)
    setBusy(true)
    try {
      const { message } = await confirmEmailChange(token)
      setOk(message)
    } catch (x) {
      setErr(x instanceof Error ? x.message : 'confirmation failed')
    } finally {
      setBusy(false)
    }
  }

  return (
    <main>
      <h1>Confirm email change</h1>
      <p>Open the link from your confirmation email, or paste the token below.</p>
      <form onSubmit={onSubmit} data-testid="confirm-email-change-form">
        {err && (
          <p role="alert" data-testid="confirm-email-change-error">
            {err}
          </p>
        )}
        {ok && (
          <p data-testid="confirm-email-change-ok">
            {ok} <Link to="/login">Go to login</Link>
          </p>
        )}
        <label>
          Confirmation token
          <input
            name="token"
            type="text"
            value={token}
            onChange={(e) => setToken(e.target.value)}
            required
            autoComplete="off"
            data-testid="confirm-email-change-token"
          />
        </label>
        <button type="submit" disabled={busy} data-testid="confirm-email-change-submit">
          {busy ? 'Confirming…' : 'Confirm new email'}
        </button>
      </form>
      <p>
        <Link to="/account">Back to account</Link>
        {' · '}
        <Link to="/login">Login</Link>
      </p>
    </main>
  )
}
