// The .ics output shows no past results and no result Sync; returning to
// Google Calendar shows them again with the explicit deselection an earlier
// step made kept.
async page => {
  const fail = message => {
    throw new Error(`soccer past results proof (mode separation): ${message}`)
  }
  const deselected = await page.$$eval('[data-game-checkbox][data-game-group="past-results"]:not(:checked)', inputs =>
    inputs.map(input => input.value)
  )
  if (deselected.length !== 1) fail(`expected one deselected past result before switching, found ${deselected.join(',')}`)
  const count = async () => (await page.locator('[data-selected-count][data-game-group="past-results"]').textContent())?.trim()
  const before = await count()

  const ics = page.locator('input[name="calendar_output"][value="ics"]')
  await ics.focus()
  await page.keyboard.press('Space')
  if (!(await ics.isChecked())) fail('Space did not choose the .ics output')
  if (await page.locator('#past-results-form').isVisible()) fail('past results are visible on the .ics path')
  if (await page.locator('[data-game-group="past-results"]:visible').count()) fail('a past-result control is visible on the .ics path')
  if (await page.getByText('Sync selected results').filter({ visible: true }).count()) fail('result Sync is visible on the .ics path')
  if (await page.locator('[data-soccer-output-only="google"]:visible').count()) fail('a Google-only element is visible on the .ics path')
  if (!(await page.locator('#download-button').isVisible())) fail('the .ics download is not visible')
  const icsRows = await page.$$eval('[data-game-checkbox]', inputs =>
    inputs.filter(input => input.checkVisibility()).map(input => `${input.dataset.gameGroup}:${input.value}`)
  )
  if (icsRows.some(row => !row.startsWith('upcoming-games:'))) fail(`the .ics path shows ${icsRows.join(',')}`)

  const google = page.locator('input[name="calendar_output"][value="google"]')
  await google.focus()
  await page.keyboard.press('Space')
  if (!(await google.isChecked())) fail('Space did not choose Google Calendar again')
  if (!(await page.locator('#past-results-form').isVisible())) fail('past results did not return in Google mode')
  if (await page.locator('#download-button').isVisible()) fail('the .ics download stayed visible in Google mode')
  const after = await page.$$eval('[data-game-checkbox][data-game-group="past-results"]:not(:checked)', inputs => inputs.map(input => input.value))
  if (after.join(',') !== deselected.join(',')) fail(`deselection changed across output switching: ${after.join(',')}`)
  if ((await count()) !== before) fail(`past count changed from ${before} to ${await count()}`)
  return { deselected, icsRows, before, after: await count() }
}
