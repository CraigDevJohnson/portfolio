// Output switching keeps fetched games and deselections. The preview has no
// Google configuration, so the server renders the Google option disabled; this
// step enables that radio in the page to drive the client-side switch. It
// proves the page script, not Google availability or a Google write.
async page => {
  const fail = message => {
    throw new Error(`soccer planner proof (switch output): ${message}`)
  }
  const selection = () =>
    page.$$eval('[data-game-checkbox][data-game-group="upcoming-games"]', inputs =>
      inputs.map(input => `${input.value}:${input.checked ? 'on' : 'off'}`).join(',')
    )
  const before = await selection()

  const google = page.locator('input[name="calendar_output"][value="google"]')
  await google.evaluate(input => {
    input.disabled = false
  })
  await google.check()
  if (await page.locator('#download-button').isVisible()) fail('the .ics download stayed visible in Google mode')
  if (!(await page.locator('[data-soccer-output-only="google"]:visible').count())) fail('no Google-only element appeared in Google mode')
  if ((await selection()) !== before) fail(`selection changed in Google mode: ${await selection()}`)

  await page.locator('input[name="calendar_output"][value="ics"]').check()
  if (!(await page.locator('#download-button').isVisible())) fail('the .ics download did not return')
  if (await page.locator('[data-soccer-output-only="google"]:visible').count()) fail('a Google-only element stayed visible after switching back')
  const after = await selection()
  if (after !== before) fail(`selection changed after switching back: ${after}`)
  const count = (await page.locator('[data-selected-count][data-game-group="upcoming-games"]').textContent())?.trim()
  if (count !== '3 games selected') fail(`selected count is ${count}`)
  return { before, after, count }
}
