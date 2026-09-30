// Output switching keeps fetched games and deselections for a visitor who is
// really offered Google Calendar. A signed-out visitor never is, so this step
// opens the preview's Google account (/__preview/account/soccer-google), the
// granted preview account on a server that offers Google Calendar, in the same
// tab. The public deselection of 7001 is remembered for the team set, so the
// account's lookup of the same teams starts with it; the step then switches
// to Google Calendar and back with the keyboard. The account has no Google
// connection, and its Google controls answer in the preview and never reach
// Google. It proves the page script, not a Google connection or write.
async page => {
  const fail = message => {
    throw new Error(`soccer planner proof (switch output): ${message}`)
  }
  const squash = text => (text || '').replace(/\s+/g, ' ').trim()
  const selection = () =>
    page.$$eval('[data-game-checkbox][data-game-group="upcoming-games"]', inputs =>
      inputs.map(input => `${input.value}:${input.checked ? 'on' : 'off'}`).join(',')
    )
  const count = async () => (await page.locator('[data-selected-count][data-game-group="upcoming-games"]').textContent())?.trim()

  const { origin } = new URL(page.url())
  await page.goto(`${origin}/__preview/account/soccer-google`, { waitUntil: 'domcontentloaded' })
  if (new URL(page.url()).pathname !== '/soccer') fail(`the preview entry landed on ${page.url()}`)
  const account = squash(await page.locator('nav[aria-label="Main navigation"] .site-account-email').first().textContent())
  if (account !== 'invited.visitor@example.com') fail(`navigation account is ${JSON.stringify(account)}`)
  const google = page.locator('input[name="calendar_output"][value="google"]')
  const ics = page.locator('input[name="calendar_output"][value="ics"]')
  if (await google.isDisabled()) fail('Google Calendar is not offered to the preview Google account')
  if (!(await ics.isChecked())) fail('the saved .ics output was not restored')

  const [response] = await Promise.all([
    page.waitForResponse(candidate => new URL(candidate.url()).pathname === '/soccer/fetch'),
    (async () => {
      await page.locator('#team_codes').fill('479147 479691')
      await page.locator('#team_codes').press('Enter')
    })(),
  ])
  if (response.status() !== 200) fail(`the Team ID fetch answered ${response.status()}`)
  // A fetch submitted from the keyboard moves focus to the swapped results on
  // the next animation frame; wait for it before choosing an output.
  await page.waitForFunction(
    () =>
      document.querySelectorAll('[data-game-checkbox][data-game-group="upcoming-games"]').length > 0 &&
      !document.getElementById('games-container')?.hasAttribute('aria-busy') &&
      document.activeElement?.id === 'games-container'
  )
  // The account's own fake LPS publishes 7004 after its first Pond Mint
  // United request, which an earlier proof on this launch may have made.
  const before = await selection()
  const ids = before.split(',').map(row => row.split(':')[0])
  if (!['7003,7001,7002', '7003,7001,7002,7004'].includes(ids.join(','))) fail(`rows are ${before}`)
  const remembered = ids.map(id => `${id}:${id === '7001' ? 'off' : 'on'}`).join(',')
  if (before !== remembered) fail(`selection is ${before}, want the remembered deselection ${remembered}`)
  const selected = `${ids.length - 1} games selected`
  if ((await count()) !== selected) fail(`selected count is ${await count()}, want ${selected}`)

  await google.focus()
  await page.keyboard.press('Space')
  if (!(await google.isChecked())) fail('Space did not choose the Google Calendar output')
  if (await page.locator('#download-button').isVisible()) fail('the .ics download stayed visible in Google mode')
  if (!(await page.locator('#past-results-form').isVisible())) fail('past results did not appear in Google mode')
  const googleNote = squash(await page.locator('.soccer-google-cta:visible').textContent())
  if (!googleNote.includes('Connect Google Calendar to add selected games directly or sync selected past results.')) fail(`Google note is ${JSON.stringify(googleNote)}`)
  const inGoogle = await selection()
  if (inGoogle !== before) fail(`selection changed in Google mode: ${inGoogle}`)

  await ics.focus()
  await page.keyboard.press('Space')
  if (!(await ics.isChecked())) fail('Space did not choose the .ics output again')
  if (!(await page.locator('#download-button').isVisible())) fail('the .ics download did not return')
  if (await page.locator('[data-soccer-output-only="google"]:visible').count()) fail('a Google-only element stayed visible after switching back')
  const after = await selection()
  if (after !== before) fail(`selection changed after switching back: ${after}`)
  if ((await count()) !== selected) fail(`selected count is ${await count()} after switching back`)
  return { route: '/soccer', account, before, inGoogle, after, count: await count(), googleNote }
}
