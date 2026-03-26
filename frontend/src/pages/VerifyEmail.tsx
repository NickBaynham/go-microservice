import { FormEvent, useEffect, useMemo, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { resendVerification, verifyEmail } from '../api'

export default function VerifyEmail() {
  const [params] = useSearchParams()
  const tokenFromQuery = useMemo(() => params.get('token')?.trim() ?? '', [params])

  const [token, setToken] = useState(tokenFromQuery)
  const [email, setEmail] = useState('')
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
      const { message } = await verifyEmail(token)
      setOk(message)
    } catch (x) {
      setErr(x instanceof Error ? x.message : 'verification failed')
    } finally {
      setBusy(false)
    }
  }

  async function onResend(e: FormEvent) {
    e.preventDefault()
    setErr(null)
    setOk(null)
    setBusy(true)
    try {
      const { message } = await resendVerification(email)
      setOk(message)
    } catch (x) {
      setErr(x instanceof Error ? x.message : 'resend failed')
    } finally {
      setBusy(false)
    }
  }

  return (
    <main>
      <h1>Verify your email</h1>
      <p>Open the link from your registration email, or paste the token below.</p>
      <form onSubmit={onSubmit} data-testid="verify-email-form">
        {err && (
          <p role="alert" data-testid="verify-email-error">
            {err}
          </p>
        )}
        {ok && (
          <p data-testid="verify-email-ok">
            {ok} <Link to="/login">Go to login</Link>
          </p>
        )}
        <label>
          Verification token
          <input
            name="token"
            type="text"
            value={token}
            onChange={(e) => setToken(e.target.value)}
            required
            autoComplete="off"
            data-testid="verify-email-token"
          />
        </label>
        <button type="submit" disabled={busy} data-testid="verify-email-submit">
          {busy ? 'Verifying…' : 'Verify'}
        </button>
      </form>
      <hr />
      <h2>Didn’t get the email?</h2>
      <form onSubmit={onResend} data-testid="resend-verification-form">
        <label>
          Email
          <input
            name="email"
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            required
            autoComplete="email"
            data-testid="resend-verification-email"
          />
        </label>
        <button type="submit" disabled={busy} data-testid="resend-verification-submit">
          Resend verification email
        </button>
      </form>
      <p>
        <Link to="/login">Back to login</Link>
      </p>
    </main>
  )
}
