// The signed-in account preview of About shows the active account and the
// shared Sign out control in both navigation regions, and no Sign in entry.
async page => {
  const fail = message => {
    throw new Error(`site account proof (signed-in account): ${message}`)
  }
  const { origin } = new URL(page.url())
  await page.goto(`${origin}/__preview/account/signed-in`, { waitUntil: 'domcontentloaded' })
  const regions = {}
  for (const label of ['Main navigation', 'Mobile navigation']) {
    const nav = page.locator(`nav[aria-label="${label}"]`)
    regions[label] = {
      account: ((await nav.locator('.site-account-email').textContent()) || '').trim(),
      signOut: ((await nav.locator('form[method="POST"][action="/sign-out"] button[type="submit"]').textContent()) || '').trim(),
      signInLinks: await nav.locator('a[href^="/sign-in"]').count(),
    }
    const region = regions[label]
    if (region.account !== 'invited.visitor@example.com') fail(`${label} account is ${JSON.stringify(region.account)}`)
    if (region.signOut !== 'Sign out') fail(`${label} sign-out control is ${JSON.stringify(region.signOut)}`)
    if (region.signInLinks) fail(`${label} still offers Sign in`)
  }
  const heading = ((await page.locator('h1').first().textContent()) || '').replace(/\s+/g, ' ').trim()
  if (!heading) fail('About content is missing under the signed-in account')
  return { route: new URL(page.url()).pathname, heading, regions }
}
