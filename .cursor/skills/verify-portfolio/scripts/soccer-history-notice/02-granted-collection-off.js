// A granted visitor on a server without durable collection, which is every
// environment until the activation review, can import, and the import dialog
// makes no history claim and sends no history_notice field.
async page => {
  const fail = message => {
    throw new Error(`soccer history notice proof (collection off): ${message}`)
  }
  const { origin } = new URL(page.url())
  await page.goto(`${origin}/__preview/account/soccer-granted`, { waitUntil: 'domcontentloaded' })
  await page.evaluate(() => window.sessionStorage.clear())
  await page.locator('input[name="calendar_output"][value="ics"]').check()
  await page.locator('#soccer-connections').waitFor({ state: 'visible' })
  await page.locator('#soccer-lps-connection button[data-open-login-modal]').click()
  const dialog = page.getByRole('dialog', { name: "Import Let's Play Soccer access" })
  await dialog.waitFor({ state: 'visible' })

  const state = {
    route: new URL(page.url()).pathname,
    teamIDs: await page.locator('#team_codes').count(),
    describedBy: await dialog.getAttribute('aria-describedby'),
    historyNotices: await page.locator('#soccer-history-notice').count(),
    historyFields: await page.locator('#soccer-login-form input[name="history_notice"]').count(),
    importDisabled: await dialog.getByRole('button', { name: 'Import access' }).isDisabled(),
  }
  if (state.route !== '/__preview/account/soccer-granted') fail(`route is ${state.route}`)
  if (state.teamIDs !== 1) fail('Team IDs is missing')
  if (state.describedBy !== 'soccer-login-description') fail(`dialog is described by ${JSON.stringify(state.describedBy)}`)
  if (state.historyNotices !== 0 || state.historyFields !== 0) fail('the dialog claims history collection that is off')
  if (!state.importDisabled) fail('the preview import control is live')
  return state
}
