import { useEffect, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { fetchMe, type User } from '../api'
import { clearToken, getToken } from '../auth'

export default function Account() {
  const nav = useNavigate()
  const [user, setUser] = useState<User | null>(null)
  const [err, setErr] = useState<string | null>(null)

  useEffect(() => {
    const tok = getToken()
    if (!tok) {
      nav('/login', { replace: true })
      return
    }
    let cancelled = false
    ;(async () => {
      try {
        const u = await fetchMe(tok)
        if (!cancelled) setUser(u)
      } catch (x) {
        if (!cancelled) {
          setErr(x instanceof Error ? x.message : 'failed to load profile')
          clearToken()
        }
      }
    })()
    return () => {
      cancelled = true
    }
  }, [nav])

  function logout() {
    clearToken()
    nav('/login', { replace: true })
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
        <dt>Role</dt>
        <dd data-testid="account-role">{user?.role}</dd>
      </dl>
      <button type="button" onClick={logout} data-testid="logout">
        Log out
      </button>
    </main>
  )
}
