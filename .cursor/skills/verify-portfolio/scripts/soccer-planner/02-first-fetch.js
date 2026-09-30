// First public Team ID fetch: the loading status is visible while LPS answers,
// then one row per game appears soonest first with every game selected.
async page => {
  const fail = message => {
    throw new Error(`soccer planner proof (first fetch): ${message}`)
  }
  const rowsOf = () =>
    page.$$eval('[data-game-checkbox][data-game-group="upcoming-games"]', inputs =>
      inputs.map(input => ({ id: input.value, checked: input.checked }))
    )

  // Hold the response briefly so the in-flight state can be observed.
  await page.route('**/soccer/fetch', async route => {
    await new Promise(resolve => setTimeout(resolve, 1200))
    await route.continue()
  })
  try {
    await page.locator('#team_codes').fill('479691, 479147')
    await page.locator('#team_codes').press('Enter')
    const status = page.locator('#loading-indicator')
    await status.waitFor({ state: 'visible', timeout: 1000 })
    if ((await status.getAttribute('role')) !== 'status') fail('the loading indicator is not a status region')
    if (!(await status.textContent())?.includes('Fetching schedules')) fail('the loading status lacks its message')
    if (!(await page.locator('[data-soccer-stage="review"]').isVisible())) fail('the review stage is hidden during the first fetch')
    await page.locator('[data-game-checkbox][data-game-group="upcoming-games"]').first().waitFor()
  } finally {
    await page.unroute('**/soccer/fetch')
  }

  const rows = await rowsOf()
  const ids = rows.map(row => row.id)
  if (ids.includes('7004')) fail('the preview LPS already published game 7004; relaunch the preview for a fresh run')
  if (ids.join(',') !== '7003,7001,7002') fail(`rows are ${ids.join(',')}, want 7003,7001,7002`)
  if (rows.some(row => !row.checked)) fail('an upcoming game did not begin selected')
  const count = (await page.locator('[data-selected-count][data-game-group="upcoming-games"]').textContent())?.trim()
  if (count !== '3 games selected') fail(`selected count is ${count}`)
  const selectAll = page.locator('[data-select-all][data-game-group="upcoming-games"]')
  if (!(await selectAll.isChecked()) || (await selectAll.evaluate(input => input.indeterminate))) fail('select-all does not show every game selected')
  if (!(await page.locator('#download-button').isVisible())) fail('the .ics download is not visible')
  if (await page.locator('[data-soccer-output-only="google"]:visible').count()) fail('a Google-only element is visible on the .ics path')
  if (await page.locator('#past-results-form').isVisible()) fail('past results are visible on the manual .ics path')
  return { rows, count }
}
