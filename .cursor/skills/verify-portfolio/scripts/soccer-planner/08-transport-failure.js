// A first fetch that never reaches the server still shows a visible, announced
// error in the review stage; the next fetch recovers with the saved selection.
async page => {
  const fail = message => {
    throw new Error(`soccer planner proof (transport failure): ${message}`)
  }
  await page.reload({ waitUntil: 'domcontentloaded' })
  if (!(await page.locator('input[name="calendar_output"][value="ics"]').isChecked())) fail('the saved .ics output was not restored')
  if (!(await page.locator('#team_codes').isVisible())) fail('the Team ID form is hidden after reload')
  if (await page.locator('[data-soccer-stage="review"]').isVisible()) fail('the empty review stage is visible before a fetch')

  await page.route('**/soccer/fetch', route => route.abort('internetdisconnected'))
  try {
    await page.locator('#team_codes').fill('479691,479147')
    await page.locator('#fetch-form button[type="submit"]').click()
    const failure = page.locator('#games-container [data-soccer-request-failure]')
    await failure.waitFor({ state: 'visible' })
    if ((await failure.getAttribute('role')) !== 'alert') fail('the request failure is not announced as an alert')
    if (!(await failure.textContent())?.includes('Could not fetch games')) fail('the request failure lacks its heading')
    if (!(await page.locator('[data-soccer-stage="review"]').isVisible())) fail('the review stage stayed hidden after a failed first fetch')
  } finally {
    await page.unroute('**/soccer/fetch')
  }

  await page.locator('#fetch-form button[type="submit"]').click()
  await page.waitForFunction(
    () =>
      document.querySelectorAll('[data-game-checkbox][data-game-group="upcoming-games"]').length === 4 &&
      !document.querySelector('[data-soccer-request-failure]')
  )
  const checked = await page.$$eval('[data-game-checkbox][data-game-group="upcoming-games"]', inputs =>
    Object.fromEntries(inputs.map(input => [input.value, input.checked]))
  )
  if (checked['7001'] || !checked['7004']) fail(`selection after recovery is ${JSON.stringify(checked)}`)
  return { recovered: checked }
}
