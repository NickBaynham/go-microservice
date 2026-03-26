/** Browser-exposed env (VITE_*) for optional features. */

export function turnstileSiteKey(): string | undefined {
  const v = (import.meta.env.VITE_TURNSTILE_SITE_KEY as string | undefined)?.trim()
  return v || undefined
}
