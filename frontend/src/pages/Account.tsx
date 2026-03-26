import { useEffect, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { cancelEmailChange, fetchMe, logoutUser, resendEmailChange, type User } from '../api'
import { getToken } from '../auth'

export default function Account() {
  const nav = useNavigate()
  const [user, setUser] = useState<User | null>(null)
  const [err, setErr] = useState<string | null>(null)
  const [pendingMsg, setPendingMsg] = useState<string | null>(null)
  const [pendingErr, setPendingErr] = useState<string | null>(null)
  const [pendingBusy, setPendingBusy] = useState(false)

  useEffect(() => {
    if (!getToken()) {
      nav('/login', { replace: true })
      return
    }
    let cancelled = false
    ;(async () => {
      try {
        const u = await fetchMe()
        if (!cancelled) setUser(u)
      } catch (x) {
        if (!cancelled) {
          setErr(x instanceof Error ? x.message : 'failed to load profile')
          await logoutUser()
        }
      }
    })()
    return () => {
      cancelled = true
    }
  }, [nav])

  async function logout() {
    await logoutUser()
    nav('/login', { replace: true })
  }

  async function onResendPending() {
    setPendingMsg(null)
    setPendingErr(null)
    setPendingBusy(true)
    try {
      const { message } = await resendEmailChange()
      setPendingMsg(message)
    } catch (x) {
      setPendingErr(x instanceof Error ? x.message : 'resend failed')
    } finally {
      setPendingBusy(false)
    }
  }

  async function onCancelPending() {
    setPendingMsg(null)
    setPendingErr(null)
    setPendingBusy(true)
    try {
      const u = await cancelEmailChange()
      setUser(u)
      setPendingMsg('Pending email change cancelled.')
    } catch (x) {
      setPendingErr(x instanceof Error ? x.message : 'cancel failed')
    } finally {
      setPendingBusy(false)
    }
  }

  if (!user && !err) {
    return (
      <main>
        <p data-testid="account-loading">Loading…</p>
      </main>
    )
  }

  if (err) {
    return (
      <main>
        <p role="alert" data-testid="account-error">
          {err}
        </p>
        <Link to="/login">Back to login</Link>
      </main>
    )
  }

  return (
    <main>
      <h1>Your account</h1>
      <dl data-testid="account-details">
        <dt>Name</dt>
        <dd data-testid="account-name">{user?.name}</dd>
        <dt>Email</dt>
        <dd data-testid="account-email">{user?.email}</dd>
        {user?.pending_email ? (
          <>
            <dt>Pending email</dt>
            <dd data-testid="account-pending-email">{user.pending_email}</dd>
          </>
        ) : null}
        <dt>Role</dt>
        <dd data-testid="account-role">{user?.role}</dd>
      </dl>
      {user?.pending_email ? (
        <section data-testid="account-pending-email-actions" style={{ marginBottom: '1rem' }}>
          <p>
            Check <strong>{user.pending_email}</strong> for a confirmation link, or open{' '}
            <Link to="/confirm-email-change">confirm email change</Link> with the token from that email.
          </p>
          {pendingErr && (
            <p role="alert" data-testid="account-pending-error">
              {pendingErr}
            </p>
          )}
          {pendingMsg && <p data-testid="account-pending-msg">{pendingMsg}</p>}
          <button type="button" disabled={pendingBusy} onClick={onResendPending} data-testid="resend-email-change">
            Resend confirmation email
          </button>{' '}
          <button type="button" disabled={pendingBusy} onClick={onCancelPending} data-testid="cancel-email-change">
            Cancel change
          </button>
        </section>
      ) : null}
      <button type="button" onClick={logout} data-testid="logout">
        Log out
      </button>
    </main>
  )
}
