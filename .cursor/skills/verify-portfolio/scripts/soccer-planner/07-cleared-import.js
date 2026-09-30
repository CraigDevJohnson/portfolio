// A stale imported LPS cookie is cleared during a public refetch and the
// server resets the private workflow, but the team-set deselection stays.
async page => {
  const fail = message => {
    throw new Error(`soccer planner proof (cleared import): ${message}`)
  }
  const { origin, hostname } = new URL(page.url())
  // The server scopes the imported session cookie to /soccer.
  await page.context().addCookies([{ name: 'lps_session', value: 'stale-preview-session', domain: hostname, path: '/soccer' }])

  const [response] = await Promise.all([
    page.waitForResponse(candidate => new URL(candidate.url()).pathname === '/soccer/fetch'),
    page.locator('#fetch-form button[type="submit"]').click(),
  ])
  const trigger = response.headers()['hx-trigger'] || ''
  if (!trigger.includes('soccer-workflow-reset')) fail(`the stale import did not reset the workflow; HX-Trigger was ${trigger}`)
  await page.waitForFunction(
    () =>
      document.querySelectorAll('[data-game-checkbox][data-game-group="upcoming-games"]').length === 4 &&
      !document.getElementById('games-container')?.hasAttribute('aria-busy')
  )

  const cookies = await page.context().cookies(`${origin}/soccer`)
  if (cookies.some(cookie => cookie.name === 'lps_session')) fail('the stale imported session cookie was not cleared')
  const checked = await page.$$eval('[data-game-checkbox][data-game-group="upcoming-games"]', inputs =>
    Object.fromEntries(inputs.map(input => [input.value, input.checked]))
  )
  if (checked['7001']) fail('clearing the stale import wiped the deselection of 7001')
  if (!checked['7004']) fail('game 7004 lost its selection')
  return { trigger, checked }
}
