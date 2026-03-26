const KEY = 'go-microservice-token'

export function getToken(): string | null {
  return sessionStorage.getItem(KEY)
}

export function setToken(token: string): void {
  sessionStorage.setItem(KEY, token)
}

export function clearToken(): void {
  sessionStorage.removeItem(KEY)
}
