// Deselecting the shared game updates the count and is remembered for this
// team set; refetching the same teams keeps it and selects the game LPS
// published since the first fetch.
async page => {
  const fail = message => {
    throw new Error(`soccer linked proof (deselect and refetch): ${message}`)
  }
  const count = async () => (await page.locator('[data-selected-count][data-game-group="upcoming-games"]').textContent())?.trim()
  const shared = page.locator('[data-game-checkbox][data-game-group="upcoming-games"][value="7001"]')
  await shared.focus()
  await page.keyboard.press('Space')
  if (await shared.isChecked()) fail('Space did not deselect game 7001')
  if ((await count()) !== '2 games selected') fail(`selected count after deselection is ${await count()}`)
  const indeterminate = await page.locator('[data-select-all][data-game-group="upcoming-games"]').evaluate(input => input.indeterminate)
  if (!indeterminate) fail('select-all is not indeterminate')
  const stored = await page.evaluate(() => window.sessionStorage.getItem('portfolio:soccer:selection:479147-479691'))
  if (JSON.stringify(JSON.parse(stored || 'null')) !== JSON.stringify({ version: 2, deselected: ['7001'] })) fail(`stored selection is ${stored}`)

  await page.locator('#soccer-team-select-form button[type="submit"]').click()
  await page.waitForFunction(
    () =>
      document.querySelectorAll('[data-game-checkbox][data-game-group="upcoming-games"]').length === 4 &&
      !document.getElementById('games-container')?.hasAttribute('aria-busy')
  )
  const rows = await page.$$eval('[data-game-checkbox][data-game-group="upcoming-games"]', inputs =>
    inputs.map(input => `${input.value}:${input.checked ? 'on' : 'off'}`).join(',')
  )
  if (rows !== '7003:on,7001:off,7002:on,7004:on') fail(`rows after refetch are ${rows}`)
  if ((await count()) !== '3 games selected') fail(`selected count after refetch is ${await count()}`)
  return { stored, rows, count: await count() }
}
