# End-user guide: account and password

This document is for **people using the web app** (register, sign in, reset a forgotten password). Developers should see the root [README](../README.md) and [frontend/README.md](../frontend/README.md) for running the stack locally.

## Create an account

1. Open the app in your browser (for local development this is often `http://localhost:5173`).
2. Go to **Register** and enter your name, email, and password (password must be at least 8 characters).
3. After signup, sign in on the **Login** page with the same email and password.

The **first** person to register in a new, empty database becomes an **admin**. Everyone who registers afterward is a normal **user**. You cannot choose your role at signup.

## Sign in

1. Open **Login**.
2. Enter your email and password.
3. You will be taken to **Account**, where you can see your profile.

## Forgot password

1. On **Login**, choose **Forgot password?** (or open the **Forgot password** link in the site header).
2. Enter the email address for your account.
3. The app always shows the same confirmation message, whether or not that email is registered. This protects your privacy (others cannot discover which emails have accounts).

### After you request a reset

- If an account exists, the **API sends an email** with a link. The link opens the **Set a new password** page and includes a **reset token** in the URL.
- In **development**, if the server is not configured with real email (SMTP), operators may see the reset link in **server logs** instead of receiving mail. Configure **SMTP** in production so users get real messages (see `.env.example` and the README environment table).
- Choose a new password (at least 8 characters) and submit. Then sign in on **Login** with the new password.

### If the link expires

Reset links are **short-lived** (default **60 minutes**, configurable on the server). If yours expires, request a new reset from **Forgot password**.

## Security tips

- Use a **unique password** for this app.
- **Sign out** by clearing your session: this app stores your session in the browser; closing the tab may not remove it until you clear site data or use a private window, depending on your browser.
- Never share reset links; anyone with the link can set a new password for that account until the token expires.

## Troubleshooting

| Issue | What to try |
|--------|-------------|
| “Invalid credentials” on login | Check email/password; use **Forgot password** if needed. |
| No reset email | Spam folder; confirm the address is correct; in dev, check whether SMTP is configured or logs show the link. |
| “Invalid or expired reset token” | Request a new reset; complete the form before the token expires. |
| Browser blocks the API (CORS) | This is a deployment configuration issue: the API must allow your app’s **Origin** (see `CORS_ALLOWED_ORIGINS` in the README). |

For API details (HTTP paths, JSON bodies), use **Swagger** at `/swagger/index.html` when the server runs in a non-production mode, or see the README **API Endpoints** section.
