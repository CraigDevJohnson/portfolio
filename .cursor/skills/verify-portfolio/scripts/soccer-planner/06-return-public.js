// Leave the preview's Google account and return to the public planner through
// the shared navigation. The signed-out visitor is again offered only the .ics
// output, and a public refetch of the team set keeps the remembered
// deselection of 7001 and newly published 7004 selected.
async page => {
  const fail = message => {
    throw new Error(`soccer planner proof (return to the public planner): ${message}`)
  }
  await page.context().clearCookies()
  await Promise.all([
    page.waitForURL(url => new URL(url).pathname === '/soccer', { waitUntil: 'domcontentloaded' }),
    page.locator("nav[aria-label='Main navigation'] a[data-nav-page='soccer']").click(),
  ])
  if (await page.locator('nav[aria-label="Main navigation"] .site-account-email').count()) fail('the navigation still shows the preview account')
  if (!(await page.locator('input[name="calendar_output"][value="google"]').isDisabled())) fail('Google output is choosable for the signed-out visitor')
  if (!(await page.locator('input[name="calendar_output"][value="ics"]').isChecked())) fail('the saved .ics output was not restored')

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
