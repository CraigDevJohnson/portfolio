// Open the disconnected Google fixture, choose Google Calendar with the
// keyboard, and require the separate consent card: it suggests the site
// sign-in account and offers another Google account, both inert in preview.
async page => {
  const fail = message => {
    throw new Error(`soccer Google account proof (suggested account): ${message}`)
  }
  const { origin } = new URL(page.url())
  await page.goto(`${origin}/__preview/soccer/google-disconnected`, { waitUntil: 'domcontentloaded' })
  await page.evaluate(() => window.sessionStorage.clear())
  await page.reload({ waitUntil: 'domcontentloaded' })

  // With loaded results and no saved choice the planner starts on .ics.
  const google = page.locator('input[name="calendar_output"][value="google"]')
  if (await google.isChecked()) fail('Google output was chosen before the visitor chose it')
  if (await google.isDisabled()) fail('the Google output is disabled in the Google fixture')
  await google.focus()
  await page.keyboard.press('Space')
  if (!(await google.isChecked())) fail('Space did not choose the Google output')
  await page.locator('#soccer-google-connection').waitFor({ state: 'visible' })

  const card = page.locator('#soccer-google-connection')
  const squash = text => (text || '').replace(/\s+/g, ' ').trim()
  const state = {
    route: new URL(page.url()).pathname,
    connectionState: await card.getAttribute('data-connection-state'),
    status: squash(await card.locator('.soccer-connection-status').textContent()),
    suggested: squash(await card.locator('[data-google-suggested-account]').textContent()),
    connectedAccounts: await card.locator('[data-google-account]').count(),
    connect: await card.locator('button[disabled]:visible', { hasText: 'Connect Google Calendar' }).count(),
    another: await card.locator('button[disabled]:visible', { hasText: 'Use another Google account' }).count(),
    liveConnectLinks: await page.locator('a[href^="/soccer/google/connect"]').count(),
  }
  if (state.route !== '/__preview/soccer/google-disconnected') fail(`route is ${state.route}`)
  if (state.connectionState !== 'disconnected' || state.status !== 'Not connected') fail(`card state is ${state.connectionState} / ${state.status}`)
  if (state.suggested !== 'site@example.com') fail(`suggested account is ${JSON.stringify(state.suggested)}`)
  if (state.connectedAccounts !== 0) fail('a suggestion is presented as a connected account')
  if (state.connect !== 1 || state.another !== 1) fail(`inert consent controls: connect ${state.connect}, another ${state.another}`)
  if (state.liveConnectLinks !== 0) fail('the preview offers a live Google consent link')
  return state
}
