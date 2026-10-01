// A signed-out visitor keeps the public Team ID path. Linked-player import
// needs the soccer grant, so the page offers no import and no history notice.
async page => {
  const fail = message => {
    throw new Error(`soccer history notice proof (signed out): ${message}`)
  }
  const { origin } = new URL(page.url())
  await page.goto(`${origin}/__preview/account/soccer-signed-out`, { waitUntil: 'domcontentloaded' })
  await page.evaluate(() => window.sessionStorage.clear())
  await page.locator('input[name="calendar_output"][value="ics"]').check()
  await page.locator('#soccer-connections').waitFor({ state: 'visible' })

  const squash = text => (text || '').replace(/\s+/g, ' ').trim()
  const state = {
    route: new URL(page.url()).pathname,
    teamIDs: await page.locator('#team_codes').isVisible(),
    fetchSchedules: await page.getByRole('button', { name: 'Fetch schedules' }).isVisible(),
    lpsCard: squash(await page.locator('#soccer-lps-connection').textContent()),
    importButtons: await page.locator('#soccer-lps-connection button[data-open-login-modal]:visible').count(),
    historyNotices: await page.locator('#soccer-history-notice').count(),
    historyFields: await page.locator('input[name="history_notice"]').count(),
  }
  if (state.route !== '/__preview/account/soccer-signed-out') fail(`route is ${state.route}`)
  if (!state.teamIDs || !state.fetchSchedules) fail('the public Team ID lookup is not available')
  if (!state.lpsCard.includes('Needs Soccer access')) fail(`LPS card is ${JSON.stringify(state.lpsCard)}`)
  if (state.importButtons !== 0) fail('linked-player import is offered without the soccer grant')
  if (state.historyNotices !== 0 || state.historyFields !== 0) fail('the page shows history collection without an import')
  return state
}
