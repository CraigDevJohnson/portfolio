// Open the connected Google fixture in Google mode and require the account
// Google reported as connected, distinct from the site sign-in account, with
// inert switch, change, and disconnect controls.
async page => {
  const fail = message => {
    throw new Error(`soccer Google account proof (connected account): ${message}`)
  }
  const { origin } = new URL(page.url())
  await page.goto(`${origin}/__preview/soccer/google-connected`, { waitUntil: 'domcontentloaded' })
  await page.locator('input[name="calendar_output"][value="google"]').check()
  await page.locator('#soccer-google-connection').waitFor({ state: 'visible' })

  const card = page.locator('#soccer-google-connection')
  const squash = text => (text || '').replace(/\s+/g, ' ').trim()
  const inert = label => card.locator('button[disabled]:visible', { hasText: label }).count()
  const state = {
    connectionState: await card.getAttribute('data-connection-state'),
    status: squash(await card.locator('.soccer-connection-status').textContent()),
    connected: squash(await card.locator('[data-google-account]').textContent()),
    copy: squash(await card.locator('.soccer-connection-copy').textContent()),
    destination: squash(await card.locator('#soccer-google-calendar option:checked').textContent()),
    switchToSite: await inert('Switch to site account'),
    change: await inert('Change Google account'),
    disconnect: await inert('Disconnect'),
  }
  if (state.connectionState !== 'connected' || state.status !== 'Calendar ready') fail(`card state is ${state.connectionState} / ${state.status}`)
  if (state.connected !== 'calendar@example.com') fail(`connected account is ${JSON.stringify(state.connected)}`)
  if (!state.copy.includes('Your site sign-in account, site@example.com, is a different Google account.')) fail(`card copy is ${JSON.stringify(state.copy)}`)
  if (state.destination !== 'Matchdays and travel notes (Primary)') fail(`destination is ${JSON.stringify(state.destination)}`)
  if (state.switchToSite !== 1 || state.change !== 1 || state.disconnect !== 1) {
    fail(`inert account controls: switch ${state.switchToSite}, change ${state.change}, disconnect ${state.disconnect}`)
  }
  return state
}
