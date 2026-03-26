const ACCESS = 'go-microservice-access'
const REFRESH = 'go-microservice-refresh'
/** Legacy single-token storage key (migrated on read). */
const LEGACY = 'go-microservice-token'

function migrateLegacy(): void {
  const old = sessionStorage.getItem(LEGACY)
  if (old && !sessionStorage.getItem(ACCESS)) {
    sessionStorage.setItem(ACCESS, old)
    sessionStorage.removeItem(LEGACY)
  }
}

/** Returns the current access (Bearer) JWT, if any. */
export function getToken(): string | null {
  migrateLegacy()
  return sessionStorage.getItem(ACCESS)
}

/** Stores only the access token (no refresh); prefer setSession after login. */
export function setToken(token: string): void {
  sessionStorage.setItem(ACCESS, token)
}

export function getRefreshToken(): string | null {
  migrateLegacy()
  return sessionStorage.getItem(REFRESH)
}

export function setSession(access: string, refresh: string): void {
  sessionStorage.setItem(ACCESS, access)
  sessionStorage.setItem(REFRESH, refresh)
  sessionStorage.removeItem(LEGACY)
}

export function clearToken(): void {
  sessionStorage.removeItem(ACCESS)
  sessionStorage.removeItem(REFRESH)
  sessionStorage.removeItem(LEGACY)
}
