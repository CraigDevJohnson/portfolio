// Open the preview's granted account, choose Google Calendar with the
// keyboard, and look up both preview teams by Team ID. Only a visitor with the
// soccer grant has Google mode, so a public lookup shows no past results. The
// review lists every scored past game once, newest first and selected,
// including one from over a year ago, and leaves out the postponed game
// without a score. The preview has no Google configuration, so its Google
// option is disabled; this step enables that radio in the page to drive the
// client-side output switch. It proves the review, not Google availability or
// a Google write.
async page => {
  const fail = message => {
    throw new Error(`soccer past results proof (Team IDs in Google mode): ${message}`)
  }
  const squash = text => (text || '').replace(/\s+/g, ' ').trim()

  const { origin } = new URL(page.url())
  await page.goto(`${origin}/__preview/account/soccer-linked`, { waitUntil: 'domcontentloaded' })
  await page.evaluate(() => window.sessionStorage.clear())
  await page.reload({ waitUntil: 'domcontentloaded' })
  if (new URL(page.url()).pathname !== '/soccer') fail(`the preview entry landed on ${page.url()}`)
  const account = squash(await page.locator('nav[aria-label="Main navigation"] .site-account-email').first().textContent())
  if (account !== 'invited.visitor@example.com') fail(`navigation account is ${JSON.stringify(account)}`)
  const google = page.locator('input[name="calendar_output"][value="google"]')
  await google.evaluate(input => {
    input.disabled = false
  })
  await google.focus()
  await page.keyboard.press('Space')
  if (!(await google.isChecked())) fail('Space did not choose the Google Calendar output')

  const [response] = await Promise.all([
    page.waitForResponse(candidate => new URL(candidate.url()).pathname === '/soccer/fetch'),
    (async () => {
      await page.locator('#team_codes').fill('479691, 479147')
      await page.locator('#team_codes').press('Enter')
    })(),
  ])
  if (response.status() !== 200) fail(`the Team ID fetch answered ${response.status()}`)
  await page.waitForFunction(
    () =>
      document.querySelectorAll('[data-game-checkbox][data-game-group="past-results"]').length > 0 &&
      !document.getElementById('games-container')?.hasAttribute('aria-busy')
  )

  const rows = await page.$$eval('[data-game-checkbox][data-game-group="past-results"]', inputs =>
    inputs.map(input => ({
      id: input.value,
      checked: input.checked,
      kickoff: input.closest('li')?.querySelector('.soccer-match-datetime')?.textContent.replace(/\s+/g, ' ').trim(),
    }))
  )
  const ids = rows.map(row => row.id).join(',')
  if (ids !== '7000,6998,6990') fail(`past results are ${ids}, want every scored past game newest first 7000,6998,6990`)
  if (rows.some(row => !row.checked)) fail('a scored past result did not begin selected')
  if (await page.locator('[data-game-checkbox][value="6995"]').count()) fail('postponed game 6995 without a score was offered')
  if (!(await page.locator('#past-results-form').isVisible())) fail('past results are hidden in Google mode')
  const heading = squash(await page.locator('#past-results-heading').textContent())
  if (heading !== 'Past results 3 games') fail(`past results heading is ${JSON.stringify(heading)}`)
  const count = (await page.locator('[data-selected-count][data-game-group="past-results"]').textContent())?.trim()
  if (count !== '3 games selected') fail(`past selected count is ${count}`)
  const selectAll = page.locator('[data-select-all][data-game-group="past-results"]')
  if (!(await selectAll.isChecked()) || (await selectAll.evaluate(input => input.indeterminate))) fail('past select-all does not show every result selected')
  if (await page.locator('#download-button').isVisible()) fail('the .ics download is visible in Google mode')
  if (await page.locator('[data-game-action][data-game-group="past-results"]').count()) fail('Sync is offered without a Google connection')
  const googleNote = squash(await page.locator('.soccer-google-cta:visible').textContent())
  if (!googleNote.includes('Google Calendar add is unavailable in this environment')) fail(`Google note is ${JSON.stringify(googleNote)}`)
  const scope = await page.locator('[data-team-fingerprint]').getAttribute('data-team-fingerprint')
  if (scope !== '479147-479691') fail(`team-set scope is ${scope}`)
  return { route: '/soccer', account, rows, heading, count, scope, googleNote }
}
