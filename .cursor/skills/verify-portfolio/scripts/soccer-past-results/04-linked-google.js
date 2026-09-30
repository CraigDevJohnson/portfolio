// The linked-player source reviews the same scored past games. The browser
// reopens the preview's Google account, chooses Google Calendar with the
// keyboard, imports a fake token through the real dialog (never a real JWT),
// keeps both players and their teams, and fetches. Selection memory is
// cleared first, because the Team ID steps used the same team set.
async page => {
  const fail = message => {
    throw new Error(`soccer past results proof (linked players in Google mode): ${message}`)
  }
  const squash = text => (text || '').replace(/\s+/g, ' ').trim()
  const encode = value => btoa(JSON.stringify(value)).replace(/=+$/, '').replace(/\+/g, '-').replace(/\//g, '_')
  const fakeJWT = [encode({ alg: 'none', typ: 'JWT' }), encode({ exp: Math.floor(Date.now() / 1000) + 3600 }), 'preview-signature'].join('.')

  const { origin } = new URL(page.url())
  await page.goto(`${origin}/__preview/account/soccer-google`, { waitUntil: 'domcontentloaded' })
  await page.evaluate(() => window.sessionStorage.clear())
  await page.reload({ waitUntil: 'domcontentloaded' })
  if (new URL(page.url()).pathname !== '/soccer') fail(`the preview entry landed on ${page.url()}`)
  const account = squash(await page.locator('nav[aria-label="Main navigation"] .site-account-email').first().textContent())
  if (account !== 'invited.visitor@example.com') fail(`navigation account is ${JSON.stringify(account)}`)

  const google = page.locator('input[name="calendar_output"][value="google"]')
  if (await google.isDisabled()) fail('Google Calendar is not offered to the preview Google account')
  await google.focus()
  await page.keyboard.press('Space')
  if (!(await google.isChecked())) fail('Space did not choose the Google Calendar output')

  await page.locator('#soccer-lps-connection button[data-open-login-modal]').click()
  const dialog = page.locator('#soccer-login-modal')
  await dialog.waitFor({ state: 'visible' })
  await page.locator('#soccer-import-jwt').fill(fakeJWT)
  await page.locator('#soccer-login-form button[type="submit"]').click()
  await page.locator('#soccer-player-select-form input[name="player_ids"]').first().waitFor({ state: 'visible' })
  await dialog.waitFor({ state: 'hidden' })

  const players = await page.$$eval('#soccer-player-select-form input[name="player_ids"]', inputs => inputs.map(input => `${input.value}:${input.checked}`))
  if (players.join(',') !== '1669080:true,1669081:true') fail(`linked players are ${players.join(',')}`)
  const [discovery] = await Promise.all([
    page.waitForResponse(candidate => new URL(candidate.url()).pathname === '/soccer/discover-teams'),
    page.locator('#soccer-player-select-form button[type="submit"]').click(),
  ])
  if (discovery.status() !== 200) fail(`team discovery answered ${discovery.status()}`)
  await page.locator('#soccer-team-select-form').waitFor({ state: 'visible' })
  const teams = await page.$$eval('#soccer-team-select-form input[name="team_ids"]', inputs =>
    inputs.map(input => `${input.getAttribute('aria-label')}:${input.checked}`)
  )
  if (teams.join('|') !== 'Pond Mint United:true|Campfire Rovers:true') fail(`linked teams are ${teams.join('|')}`)

  const [fetched] = await Promise.all([
    page.waitForResponse(candidate => new URL(candidate.url()).pathname === '/soccer/fetch'),
    page.locator('#soccer-team-select-form button[type="submit"]').click(),
  ])
  if (fetched.status() !== 200) fail(`the linked fetch answered ${fetched.status()}`)
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
  const count = (await page.locator('[data-selected-count][data-game-group="past-results"]').textContent())?.trim()
  if (count !== '3 games selected') fail(`past selected count is ${count}`)
  const selectAll = page.locator('[data-select-all][data-game-group="past-results"]')
  if (!(await selectAll.isChecked()) || (await selectAll.evaluate(input => input.indeterminate))) fail('past select-all does not show every result selected')
  if (await page.locator('#download-button').isVisible()) fail('the .ics download is visible in Google mode')
  if (await page.locator('[data-game-action][data-game-group="past-results"]').count()) fail('Sync is offered without a Google connection')
  const hidden = await page.$$eval('#past-results-form input[type="hidden"]', inputs => inputs.map(input => `${input.name}=${input.value}`))
  if (hidden.join('&') !== 'team_codes=479691,479147&player_ids=1669080&player_ids=1669081') fail(`past results carry ${hidden.join('&')}`)
  const scope = await page.locator('[data-team-fingerprint]').getAttribute('data-team-fingerprint')
  if (scope !== '479147-479691') fail(`team-set scope is ${scope}`)
  return { route: '/soccer', account, teams, rows, count, hidden, scope }
}
