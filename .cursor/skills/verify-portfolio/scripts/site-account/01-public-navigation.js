// From Home, follow every public destination in the shared desktop
// navigation. Each page must load without sign-in, mark its own link current,
// and, because the preview launch configures no site sign-in, show neither an
// account nor a sign-in entry.
async page => {
  const fail = message => {
    throw new Error(`site account proof (public navigation): ${message}`)
  }
  const { origin } = new URL(page.url())
  await page.goto(`${origin}/`, { waitUntil: 'domcontentloaded' })
  const nav = page.locator('nav[aria-label="Main navigation"]')
  const visited = []
  for (const path of ['/about', '/experience', '/skills', '/projects', '/education', '/contact', '/soccer', '/']) {
    const link = nav.locator(`a.nav-link[href="${path}"]`)
    if ((await link.count()) !== 1) fail(`navigation has ${await link.count()} links to ${path}`)
    const [response] = await Promise.all([
      page.waitForResponse(r => r.request().resourceType() === 'document' && new URL(r.url()).pathname === path),
      link.click(),
    ])
    await page.waitForURL(`${origin}${path}`)
    const heading = ((await page.locator('h1').first().textContent()) || '').replace(/\s+/g, ' ').trim()
    const current = await nav.locator('[aria-current="page"]').evaluateAll(links => links.map(a => a.getAttribute('href')))
    if (response.status() !== 200) fail(`${path} returned ${response.status()}`)
    if (!heading) fail(`${path} has no visible heading`)
    if (current.length !== 1 || current[0] !== path) fail(`${path} marks ${JSON.stringify(current)} current`)
    if (await nav.locator('.site-account-email, a[href^="/sign-in"]').count()) fail(`${path} advertises account state while site sign-in is not configured`)
    visited.push(`${path} -> ${response.status()} "${heading}"`)
  }
  return visited
}
