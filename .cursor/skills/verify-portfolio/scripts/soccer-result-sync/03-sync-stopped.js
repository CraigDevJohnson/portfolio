// Open the fixture for a Sync that Google stopped partway, for a usage
// limit. The alert reports the result already written and asks for a retry,
// which leaves current results unchanged.
async page => {
  const fail = message => {
    throw new Error(`soccer result sync proof (stopped): ${message}`)
  }
  const squash = text => (text || '').replace(/\s+/g, ' ').trim()
  const { origin } = new URL(page.url())
  await page.evaluate(() => window.sessionStorage.clear())
  await page.goto(`${origin}/__preview/soccer/google-sync-error`, { waitUntil: 'domcontentloaded' })
  await page.locator('input[name="calendar_output"][value="google"]').check()
  await page.locator('#past-results-form').waitFor({ state: 'visible' })

  const feedback = page.locator('#past-results-feedback')
  const state = {
    role: await feedback.locator('.ui-feedback').getAttribute('role'),
    title: squash(await feedback.locator('.ui-feedback-title').textContent()),
    message: squash(await feedback.locator('.ui-feedback-message').textContent()),
  }
  if (state.role !== 'alert' || state.title !== 'Selected results were not synced') fail(`alert is role ${state.role}, title ${JSON.stringify(state.title)}`)
  if (state.message !== '1 game result(s) updated in Google Calendar. Skipped 0 game(s). Could not finish result sync. Retry later; results already current will be left unchanged.') {
    fail(`alert is ${JSON.stringify(state.message)}`)
  }
  return state
}
