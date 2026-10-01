// Leave the preview's Google account and return to the public planner through
// the shared navigation. The signed-out visitor is again offered only the .ics
// output, and a public refetch of the team set keeps the remembered
// deselection of 7001 and newly published 7004 selected.
async page => {
  const fail = message => {
    throw new Error(`soccer planner proof (return to the public planner): ${message}`)
  }
  await page.context().clearCookies()
  // The tab is already on /soccer as the Google account, so a URL wait would
  // return at once. Wait for the main frame to commit the new document, then
  // for that document's DOMContentLoaded, by which the head's HTMX and the
  // deferred main.js have run.
  await Promise.all([
    page.waitForEvent('framenavigated', frame => frame === page.mainFrame()),
    page.locator("nav[aria-label='Main navigation'] a[data-nav-page='soccer']").click(),
  ])
  await page.waitForLoadState('domcontentloaded')
  if (new URL(page.url()).pathname !== '/soccer') fail(`the navigation landed on ${page.url()}`)
  if ((await page.evaluate(() => typeof window.htmx)) === 'undefined') fail('HTMX did not load on the public planner')
  await page
    .waitForFunction(() => document.querySelector('input[name="calendar_output"][value="ics"]')?.checked, null, { timeout: 5000 })
    .catch(() => fail('the saved .ics output was not restored'))
  if (await page.locator('nav[aria-label="Main navigation"] .site-account-email').count()) fail('the navigation still shows the preview account')
  if (!(await page.locator('input[name="calendar_output"][value="google"]').isDisabled())) fail('Google output is choosable for the signed-out visitor')

  const [response] = await Promise.all([
    page.waitForResponse(candidate => new URL(candidate.url()).pathname === '/soccer/fetch'),
    (async () => {
      await page.locator('#team_codes').fill('479147 479691')
      await page.locator('#fetch-form button[type="submit"]').click()
    })(),
  ])
  if (response.status() !== 200) fail(`the public refetch answered ${response.status()}`)
  await page.waitForFunction(
    () =>
      document.querySelectorAll('[data-game-checkbox][data-game-group="upcoming-games"]').length === 4 &&
      !document.getElementById('games-container')?.hasAttribute('aria-busy')
  )
  const selection = await page.$$eval('[data-game-checkbox][data-game-group="upcoming-games"]', inputs =>
    inputs.map(input => `${input.value}:${input.checked ? 'on' : 'off'}`).join(',')
  )
  if (selection !== '7003:on,7001:off,7002:on,7004:on') fail(`public selection is ${selection}`)
  const count = (await page.locator('[data-selected-count][data-game-group="upcoming-games"]').textContent())?.trim()
  if (count !== '3 games selected') fail(`selected count is ${count}`)
  if (!(await page.locator('#download-button').isVisible())) fail('the .ics download is not visible')
  if (await page.locator('[data-soccer-output-only="google"]:visible').count()) fail('a Google-only element is visible to the signed-out visitor')
  return { route: '/soccer', selection, count }
}
