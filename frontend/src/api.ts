import { clearToken, getRefreshToken, getToken, setSession, setToken } from './auth'

/** API base without trailing slash. From Vite env at build time. */
export function apiBase(): string {
  const v = import.meta.env.VITE_API_BASE_URL as string | undefined
  return (v ?? '').replace(/\/$/, '')
}

export type ApiError = { error: string }

export type User = {
  id?: string
  name: string
  email: string
  pending_email?: string
  role: string
  email_verified?: boolean
  created_at?: string
  updated_at?: string
}

export async function registerUser(body: {
  name: string
  email: string
  password: string
  turnstile_token?: string
}): Promise<{ message: string; user: User }> {
  const res = await fetch(`${apiBase()}/auth/register`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  const data = (await res.json()) as { message?: string; user?: User; error?: string }
  if (!res.ok) {
    throw new Error(data.error ?? `register failed (${res.status})`)
  }
  return { message: data.message ?? '', user: data.user as User }
}

export async function verifyEmail(token: string): Promise<{ message: string }> {
  const res = await fetch(`${apiBase()}/auth/verify-email`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ token }),
  })
  const data = (await res.json()) as { message?: string; error?: string }
  if (!res.ok) {
    throw new Error(data.error ?? `verify-email failed (${res.status})`)
  }
  return { message: data.message ?? '' }
}

export async function resendVerification(email: string): Promise<{ message: string }> {
  const res = await fetch(`${apiBase()}/auth/resend-verification`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email }),
  })
  const data = (await res.json()) as { message?: string; error?: string }
  if (!res.ok) {
    throw new Error(data.error ?? `resend-verification failed (${res.status})`)
  }
  return { message: data.message ?? '' }
}

export async function loginUser(
  email: string,
  password: string,
  turnstileToken?: string,
): Promise<{ token: string; user: User }> {
  const res = await fetch(`${apiBase()}/auth/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      email,
      password,
      ...(turnstileToken ? { turnstile_token: turnstileToken } : {}),
    }),
  })
  const data = (await res.json()) as {
    token?: string
    access_token?: string
    refresh_token?: string
    user?: User
    error?: string
  }
  if (!res.ok) {
    throw new Error(data.error ?? `login failed (${res.status})`)
  }
  const access = data.access_token ?? data.token ?? ''
  if (!access) {
    throw new Error('login response missing access token')
  }
  if (data.refresh_token) {
    setSession(access, data.refresh_token)
  } else {
    setToken(access)
  }
  return { token: access, user: data.user as User }
}

/** Exchanges the stored refresh token for a new access + refresh pair. Returns null if refresh fails. */
export async function refreshSession(): Promise<string | null> {
  const rt = getRefreshToken()
  if (!rt) return null
  const res = await fetch(`${apiBase()}/auth/refresh`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ refresh_token: rt }),
  })
  const data = (await res.json()) as {
    access_token?: string
    refresh_token?: string
    error?: string
  }
  if (!res.ok || !data.access_token || !data.refresh_token) {
    return null
  }
  setSession(data.access_token, data.refresh_token)
  return data.access_token
}

export async function fetchMe(accessToken?: string): Promise<User> {
  let token = accessToken ?? getToken()
  if (!token) {
    throw new Error('not authenticated')
  }

  const req = (t: string) =>
    fetch(`${apiBase()}/me`, {
      headers: { Authorization: `Bearer ${t}` },
    })

  let res = await req(token)
  if (res.status === 401) {
    const t2 = await refreshSession()
    if (t2) {
      res = await req(t2)
    }
  }

  const data = (await res.json()) as User & ApiError
  if (!res.ok) {
    throw new Error(data.error ?? `me failed (${res.status})`)
  }
  return data as User
}

/** Revokes the refresh token on the server and clears local session storage. */
export async function logoutUser(): Promise<void> {
  const rt = getRefreshToken()
  if (rt) {
    try {
      await fetch(`${apiBase()}/auth/logout`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ refresh_token: rt }),
      })
    } catch {
      /* network errors: still clear client session */
    }
  }
  clearToken()
}

export async function requestPasswordReset(email: string): Promise<{ message: string }> {
  const res = await fetch(`${apiBase()}/auth/forgot-password`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email }),
  })
  const data = (await res.json()) as { message?: string; error?: string }
  if (!res.ok) {
    throw new Error(data.error ?? `forgot-password failed (${res.status})`)
  }
  return { message: data.message ?? '' }
}

export async function resetPassword(token: string, password: string): Promise<{ message: string }> {
  const res = await fetch(`${apiBase()}/auth/reset-password`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ token, password }),
  })
  const data = (await res.json()) as { message?: string; error?: string }
  if (!res.ok) {
    throw new Error(data.error ?? `reset-password failed (${res.status})`)
  }
  return { message: data.message ?? '' }
}

export async function confirmEmailChange(token: string): Promise<{ message: string }> {
  const res = await fetch(`${apiBase()}/auth/confirm-email-change`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ token }),
  })
  const data = (await res.json()) as { message?: string; error?: string }
  if (!res.ok) {
    throw new Error(data.error ?? `confirm-email-change failed (${res.status})`)
  }
  return { message: data.message ?? '' }
}

async function postWithAuth(path: string, jsonBody?: object): Promise<Response> {
  let token = getToken()
  if (!token) {
    throw new Error('not authenticated')
  }
  const run = (t: string) =>
    fetch(`${apiBase()}${path}`, {
      method: 'POST',
      headers: {
        Authorization: `Bearer ${t}`,
        ...(jsonBody !== undefined ? { 'Content-Type': 'application/json' } : {}),
      },
      body: jsonBody !== undefined ? JSON.stringify(jsonBody) : undefined,
    })
  let res = await run(token)
  if (res.status === 401) {
    const t2 = await refreshSession()
    if (t2) {
      res = await run(t2)
    }
  }
  return res
}

export async function resendEmailChange(): Promise<{ message: string }> {
  const res = await postWithAuth('/me/resend-email-change')
  const data = (await res.json()) as { message?: string; error?: string }
  if (!res.ok) {
    throw new Error(data.error ?? `resend-email-change failed (${res.status})`)
  }
  return { message: data.message ?? '' }
}

export async function cancelEmailChange(): Promise<User> {
  const res = await postWithAuth('/me/cancel-email-change')
  const data = (await res.json()) as User & ApiError
  if (!res.ok) {
    throw new Error(data.error ?? `cancel-email-change failed (${res.status})`)
  }
  return data as User
}
