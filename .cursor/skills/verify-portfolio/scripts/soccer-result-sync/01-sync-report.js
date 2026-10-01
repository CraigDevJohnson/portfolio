// Open the result Sync success fixture in Google mode. Before the visitor
// acts, the past results say Sync writes only into events this site added
// and never adds a past game; after Sync, the report gives the updated and
// skipped counts and why the skipped game was left alone. The fixture renders
// the report the handler writes; it does not call Google.
async page => {
  const fail = message => {
    throw new Error(`soccer result sync proof (report): ${message}`)
  }
  const squash = text => (text || '').replace(/\s+/g, ' ').trim()
  const { origin } = new URL(page.url())
  await page.evaluate(() => window.sessionStorage.clear())
  await page.goto(`${origin}/__preview/soccer/google-sync-success`, { waitUntil: 'domcontentloaded' })
  const google = page.locator('input[name="calendar_output"][value="google"]')
  await google.focus()
  await page.keyboard.press('Space')
  if (!(await google.isChecked())) fail('Space did not choose the Google Calendar output')
  await page.locator('#past-results-form').waitFor({ state: 'visible' })

  const form = page.locator('#past-results-form')
  const feedback = page.locator('#past-results-feedback')
  const sync = form.locator('[data-game-action][data-game-group="past-results"]')
  const state = {
    path: new URL(page.url()).pathname,
    copy: squash(await form.locator('.games-section-copy').textContent()),
    rows: await form.locator('[data-game-checkbox][data-game-group="past-results"]:checked').count(),
    sync: squash(await sync.textContent()),
    syncDisabled: await sync.isDisabled(),
    feedbackRole: await feedback.locator('.ui-feedback').getAttribute('role'),
    feedbackTitle: squash(await feedback.locator('.ui-feedback-title').textContent()),
    feedback: squash(await feedback.locator('.ui-feedback-message').textContent()),
    feedbackVisible: await feedback.isVisible(),
  }
  if (state.path !== '/__preview/soccer/google-sync-success') fail(`the fixture is at ${state.path}`)
  if (state.copy !== 'Completed matches stay available here. Sync writes each selected score into the event this site added to your chosen calendar and never adds a past game.') {
    fail(`past results copy is ${JSON.stringify(state.copy)}`)
  }
  if (state.rows !== 2) fail(`${state.rows} past results are selected, want 2`)
  if (state.sync !== 'Sync selected results' || !state.syncDisabled) fail(`Sync control is ${JSON.stringify(state.sync)}, disabled ${state.syncDisabled}`)
  if (!state.feedbackVisible || state.feedbackRole !== 'status' || state.feedbackTitle !== 'Selected results synced') {
    fail(`report region is visible ${state.feedbackVisible}, role ${state.feedbackRole}, title ${JSON.stringify(state.feedbackTitle)}`)
  }
  if (state.feedback !== '1 game result(s) updated in Google Calendar. Skipped 1 game(s): 1 unmatched (no event this site added).') {
    fail(`report is ${JSON.stringify(state.feedback)}`)
  }
  return state
}
