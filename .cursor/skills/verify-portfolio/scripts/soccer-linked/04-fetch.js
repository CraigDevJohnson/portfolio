// Fetching the chosen teams shows the shared game once, soonest first, with
// every upcoming game selected. Past results stay out of the .ics path.
async page => {
  const fail = message => {
    throw new Error(`soccer linked proof (fetch): ${message}`)
  }
  await page.locator('#soccer-team-select-form button[type="submit"]').click()
  await page.waitForFunction(
    () =>
      document.querySelectorAll('[data-game-checkbox][data-game-group="upcoming-games"]').length > 0 &&
      !document.getElementById('games-container')?.hasAttribute('aria-busy')
  )
  const rows = await page.$$eval('[data-game-checkbox][data-game-group="upcoming-games"]', inputs =>
    inputs.map(input => ({ id: input.value, checked: input.checked }))
  )
  const ids = rows.map(row => row.id).join(',')
  if (ids.includes('7004')) fail('the preview LPS already published game 7004; relaunch the preview for a fresh run')
  if (ids !== '7003,7001,7002') fail(`rows are ${ids}, want 7003,7001,7002`)
  if (rows.some(row => !row.checked)) fail('an upcoming game did not begin selected')
  const count = (await page.locator('[data-selected-count][data-game-group="upcoming-games"]').textContent())?.trim()
  if (count !== '3 games selected') fail(`selected count is ${count}`)
  const scope = await page.locator('[data-team-fingerprint]').getAttribute('data-team-fingerprint')
  if (scope !== '479147-479691') fail(`team-set scope is ${scope}`)
  if (!(await page.locator('[data-soccer-stage="review"]').isVisible())) fail('the review stage is hidden')
  if (await page.locator('[data-game-group="past-results"]:visible').count()) fail('past results are visible on the .ics path')
  if (!(await page.locator('#download-button').isVisible())) fail('the .ics download is not visible')
  return { rows, count, scope }
}
