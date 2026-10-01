// Open the paused-destination Google fixture in Google mode: the chosen
// calendar left the account, so the card asks for a new writable calendar
// instead of presenting a ready destination or falling back to primary.
async page => {
  const fail = message => {
    throw new Error(`soccer Google account proof (paused destination): ${message}`)
  }
  const { origin } = new URL(page.url())
  await page.goto(`${origin}/__preview/soccer/google-calendar-paused`, { waitUntil: 'domcontentloaded' })
  await page.locator('input[name="calendar_output"][value="google"]').check()
  await page.locator('#soccer-google-connection').waitFor({ state: 'visible' })

  const card = page.locator('#soccer-google-connection')
  const squash = text => (text || '').replace(/\s+/g, ' ').trim()
  const select = card.locator('#soccer-google-calendar')
  const state = {
    route: new URL(page.url()).pathname,
    connectionState: await card.getAttribute('data-connection-state'),
    status: squash(await card.locator('.soccer-connection-status').textContent()),
    connected: squash(await card.locator('[data-google-account]').textContent()),
    copy: squash(await card.locator('.soccer-connection-copy').textContent()),
    destination: squash(await select.locator('option:checked').textContent()),
    placeholderDisabled: (await select.locator('option:checked').getAttribute('disabled')) !== null,
    choices: (await select.locator('option:not([disabled])').allTextContents()).map(squash),
    readySummaries: await card.locator('.soccer-connection-meta').count(),
  }
  if (state.route !== '/__preview/soccer/google-calendar-paused') fail(`route is ${state.route}`)
  if (state.connectionState !== 'connected' || state.status !== 'Calendar selection needed') fail(`card state is ${state.connectionState} / ${state.status}`)
  if (state.connected !== 'calendar@example.com') fail(`connected account is ${JSON.stringify(state.connected)}`)
  if (!state.copy.includes('Writes are paused until you save a destination.') || state.copy.includes('Calendar ready')) fail(`card copy is ${JSON.stringify(state.copy)}`)
  if (state.destination !== 'Choose a writable calendar' || !state.placeholderDisabled) fail(`destination is ${JSON.stringify(state.destination)} (placeholder disabled: ${state.placeholderDisabled})`)
  if (!state.choices.includes('Matchdays and travel notes (Primary)') || state.choices.length !== 2) fail(`writable choices are ${JSON.stringify(state.choices)}`)
  if (state.readySummaries !== 0) fail('the card still summarizes a ready destination')

  await page.setViewportSize({ width: 390, height: 844 })
  await select.scrollIntoViewIfNeeded()
  state.narrow = await page.evaluate(() => {
    const control = document.querySelector('#soccer-google-calendar').getBoundingClientRect()
    return {
      viewport: window.innerWidth,
      document: document.documentElement.scrollWidth,
      selectLeft: Math.round(control.left),
      selectRight: Math.round(control.right),
    }
  })
  if (state.narrow.document > state.narrow.viewport) fail(`page scrolls horizontally: ${state.narrow.document}px in a ${state.narrow.viewport}px viewport`)
  if (state.narrow.selectLeft < 0 || state.narrow.selectRight > state.narrow.viewport) fail(`destination choice is clipped: ${JSON.stringify(state.narrow)}`)
  return state
}
