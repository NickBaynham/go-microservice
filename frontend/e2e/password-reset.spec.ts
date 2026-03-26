import { expect, test } from '@playwright/test'

test.describe('password reset', () => {
  test('forgot + reset + login', async ({ page, request }) => {
    const apiBase = process.env.E2E_API_BASE_URL || 'http://127.0.0.1:18080'
    const suffix = Date.now()
    const email = `e2e.reset.${suffix}@example.com`
    const oldPassword = 'e2e-old-pass-1'
    const newPassword = 'e2e-new-pass-2'
    const name = 'E2E Reset User'

    await page.goto('/register')
    await page.getByTestId('register-name').fill(name)
    await page.getByTestId('register-email').fill(email)
    await page.getByTestId('register-password').fill(oldPassword)
    await page.getByTestId('register-submit').click()
    await page.waitForURL('**/login', { timeout: 15_000 })

    const forgotRes = await request.post(`${apiBase}/auth/forgot-password`, {
      data: { email },
    })
    expect(forgotRes.ok()).toBeTruthy()
    const forgotBody = (await forgotRes.json()) as { reset_token?: string }
    test.skip(
      !forgotBody.reset_token,
      'Run against Docker e2e API (ENV=test) so forgot-password returns reset_token — e.g. make e2e-up',
    )

    await page.goto(`/reset-password?token=${encodeURIComponent(forgotBody.reset_token!)}`)
    await expect(page.getByRole('heading', { name: 'Set a new password' })).toBeVisible()
    await page.getByTestId('reset-password').fill(newPassword)
    await page.getByTestId('reset-submit').click()
    await page.waitForURL('**/login', { timeout: 15_000 })
    await expect(page.getByTestId('login-flash-reset')).toBeVisible()

    await page.getByTestId('login-email').fill(email)
    await page.getByTestId('login-password').fill(newPassword)
    await page.getByTestId('login-submit').click()
    await page.waitForURL('**/account', { timeout: 15_000 })
    await expect(page.getByTestId('account-email')).toHaveText(email)
  })
})
