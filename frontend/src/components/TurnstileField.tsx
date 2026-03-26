import { Turnstile } from '@marsidev/react-turnstile'
import { turnstileSiteKey } from '../envPublic'

type Props = { onToken: (token: string) => void }

/** Renders Cloudflare Turnstile when VITE_TURNSTILE_SITE_KEY is set (omit when API has no TURNSTILE_SECRET_KEY). */
export default function TurnstileField({ onToken }: Props) {
  const siteKey = turnstileSiteKey()
  if (!siteKey) {
    return null
  }
  return (
    <div data-testid="turnstile-widget">
      <Turnstile siteKey={siteKey} onSuccess={onToken} />
    </div>
  )
}
