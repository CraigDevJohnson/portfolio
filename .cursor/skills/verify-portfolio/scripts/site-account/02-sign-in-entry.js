// On the signed-out account preview fixture of About, the shared navigation's
// Sign in entry carries About as its return destination in both navigation
// regions. Following it opens the sign-in landing with that destination in its
// URL. The preview has no Cognito, so the landing reports that sign-in is
// unavailable instead of offering Google, and navigation stops advertising
// sign-in. This step does not prove sign-in return: the browser never submits
// the landing form, follows the Cognito callback, or lands back on About.
async page => {
  const fail = message => {
    throw new Error(`site account proof (sign-in entry): ${message}`)
  }
  const { origin } = new URL(page.url())
  const wantHref = '/sign-in?return_to=%2Fabout'
  await page.goto(`${origin}/__preview/account/signed-out`, { waitUntil: 'domcontentloaded' })
  const hrefs = selector => page.locator(selector).evaluateAll(links => links.map(a => a.getAttribute('href')))
  const desktop = await hrefs('nav[aria-label="Main navigation"] a.site-account-link')
  const mobile = await hrefs('nav[aria-label="Mobile navigation"] a.site-account-link')
  if (desktop.length !== 1 || desktop[0] !== wantHref) fail(`desktop Sign in entry is ${JSON.stringify(desktop)}`)
  if (mobile.length !== 1 || mobile[0] !== wantHref) fail(`mobile Sign in entry is ${JSON.stringify(mobile)}`)
  if (await page.locator('.site-account-email, form[action="/sign-out"]').count()) fail('signed-out navigation shows an account')

  const [response] = await Promise.all([
    page.waitForResponse(r => r.request().resourceType() === 'document' && new URL(r.url()).pathname === '/sign-in'),
    page.locator('nav[aria-label="Main navigation"] a.site-account-link').click(),
  ])
  await page.waitForURL(`${origin}${wantHref}`)
  const landing = {
    route: new URL(page.url()).pathname + new URL(page.url()).search,
    status: response.status(),
    cacheControl: response.headers()['cache-control'],
    heading: ((await page.locator('h1').textContent()) || '').trim(),
    message: ((await page.locator('[role="status"]').first().textContent()) || '').trim(),
    returnTo: new URL(page.url()).searchParams.get('return_to'),
    signInEntries: await page.locator('nav a.site-account-link').count(),
    googleForms: await page.locator('form[action="/sign-in"]').count(),
  }
  if (landing.heading !== 'Sign in') fail(`landing heading is ${JSON.stringify(landing.heading)}`)
  if (landing.returnTo !== '/about') fail(`landing return destination is ${JSON.stringify(landing.returnTo)}`)
  if (landing.signInEntries !== 0) fail('navigation advertises sign-in that this server cannot start')
  if (landing.cacheControl !== 'no-store') fail(`landing is cacheable: ${landing.cacheControl}`)
  if (landing.status !== 503 || landing.message !== 'Site sign-in is unavailable right now.' || landing.googleForms !== 0) {
    fail(`preview landing should report unavailable sign-in: ${JSON.stringify(landing)}`)
  }
  return landing
}
