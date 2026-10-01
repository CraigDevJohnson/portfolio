// With durable collection enabled, the granted visitor's import dialog
// discloses indefinite linked-player history before any import: the notice is
// part of the dialog's description, precedes the JWT field and Import access,
// and the form carries the history_notice field that alone lets an import
// collect. Team IDs stays public beside it.
async page => {
  const fail = message => {
    throw new Error(`soccer history notice proof (collection on): ${message}`)
  }
  const { origin } = new URL(page.url())
  await page.goto(`${origin}/__preview/account/soccer-granted-history`, { waitUntil: 'domcontentloaded' })
  await page.evaluate(() => window.sessionStorage.clear())
  await page.locator('input[name="calendar_output"][value="ics"]').check()
  await page.locator('#soccer-connections').waitFor({ state: 'visible' })
  const noticeBeforeOpening = await page.locator('#soccer-history-notice').isVisible()
  await page.locator('#soccer-lps-connection button[data-open-login-modal]').click()
  const dialog = page.getByRole('dialog', { name: "Import Let's Play Soccer access" })
  await dialog.waitFor({ state: 'visible' })
  const notice = dialog.locator('#soccer-history-notice')

  const squash = text => (text || '').replace(/\s+/g, ' ').trim()
  const state = {
    route: new URL(page.url()).pathname,
    teamIDs: await page.locator('#team_codes').count(),
    noticeBeforeOpening,
    noticeVisible: await notice.isVisible(),
    notice: squash(await notice.textContent()),
    describedBy: (await dialog.getAttribute('aria-describedby')).split(/\s+/),
    order: await page.evaluate(() => {
      const notice = document.querySelector('#soccer-history-notice')
      const field = document.querySelector('#soccer-import-jwt')
      const submit = document.querySelector('#soccer-login-form button[type="submit"]')
      const precedes = (a, b) => Boolean(a.compareDocumentPosition(b) & Node.DOCUMENT_POSITION_FOLLOWING)
      return { beforeField: precedes(notice, field), beforeSubmit: precedes(notice, submit) }
    }),
    historyField: await dialog.locator('#soccer-login-form input[type="hidden"][name="history_notice"]').getAttribute('value'),
    importDisabled: await dialog.getByRole('button', { name: 'Import access' }).isDisabled(),
    liveImportForms: await page.locator('form[hx-post="/soccer/import"]').count(),
  }
  if (state.route !== '/__preview/account/soccer-granted-history') fail(`route is ${state.route}`)
  if (state.teamIDs !== 1) fail('Team IDs is missing')
  if (state.noticeBeforeOpening) fail('the notice shows outside the import dialog')
  if (!state.noticeVisible) fail('the open import dialog does not show the history notice')
  for (const disclosure of ['Kept indefinitely', 'every player linked', "including players you don't choose", 'indefinitely', 'refreshing', 'JWT stays temporary']) {
    if (!state.notice.includes(disclosure)) fail(`notice ${JSON.stringify(state.notice)} does not say ${JSON.stringify(disclosure)}`)
  }
  if (!state.describedBy.includes('soccer-history-notice')) fail(`dialog is described by ${state.describedBy.join(' ')}`)
  if (!state.order.beforeField || !state.order.beforeSubmit) fail(`notice does not precede the import: ${JSON.stringify(state.order)}`)
  if (state.historyField !== 'indefinite') fail(`history_notice field is ${JSON.stringify(state.historyField)}`)
  if (!state.importDisabled || state.liveImportForms !== 0) fail('the preview import control is live')
  return state
}
