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
  role: string
  created_at?: string
  updated_at?: string
}

export async function registerUser(body: {
  name: string
  email: string
  password: string
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

export async function loginUser(
  email: string,
  password: string,
): Promise<{ token: string; user: User }> {
  const res = await fetch(`${apiBase()}/auth/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email, password }),
  })
  const data = (await res.json()) as { token?: string; user?: User; error?: string }
  if (!res.ok) {
    throw new Error(data.error ?? `login failed (${res.status})`)
  }
  return { token: data.token as string, user: data.user as User }
}

export async function fetchMe(token: string): Promise<User> {
  const res = await fetch(`${apiBase()}/me`, {
    headers: { Authorization: `Bearer ${token}` },
  })
  const data = (await res.json()) as User & ApiError
  if (!res.ok) {
    throw new Error(data.error ?? `me failed (${res.status})`)
  }
  return data as User
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
