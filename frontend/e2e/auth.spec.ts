import { expect, test } from '@playwright/test'

test.describe('auth flow', () => {
  test('register, login, view account', async ({ page }) => {
    const suffix = Date.now()
    const email = `e2e.user.${suffix}@example.com`
    const password = 'e2e-secret-1'
    const name = 'E2E User'

    await page.goto('/register')
    await expect(page.getByRole('heading', { name: 'Sign up' })).toBeVisible()

    await page.getByTestId('register-name').fill(name)
    await page.getByTestId('register-email').fill(email)
    await page.getByTestId('register-password').fill(password)
    await page.getByTestId('register-submit').click()

    await page.waitForURL('**/login', { timeout: 15_000 })
    await expect(page.getByTestId('login-flash')).toBeVisible()

    await page.getByTestId('login-email').fill(email)
    await page.getByTestId('login-password').fill(password)
    await page.getByTestId('login-submit').click()

    await page.waitForURL('**/account', { timeout: 15_000 })
    await expect(page.getByTestId('account-email')).toHaveText(email)
    await expect(page.getByTestId('account-name')).toHaveText(name)
    await expect(page.getByTestId('account-role')).toBeVisible()
  })
})
