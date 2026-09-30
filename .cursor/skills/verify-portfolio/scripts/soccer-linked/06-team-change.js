// Changing the team set to Campfire Rovers alone fetches only its games; the
// other set's deselection does not carry over, the count follows the new
// set, and a deselection here is scoped to it.
async page => {
  const fail = message => {
    throw new Error(`soccer linked proof (team change): ${message}`)
  }
  const count = async () => (await page.locator('[data-selected-count][data-game-group="upcoming-games"]').textContent())?.trim()
  const pondMint = page.locator('#soccer-team-select-form input[name="team_ids"][value="479691"]')
  await pondMint.focus()
  await page.keyboard.press('Space')
  if (await pondMint.isChecked()) fail('Space did not deselect Pond Mint United')
  await page.locator('#soccer-team-select-form button[type="submit"]').click()
  await page.waitForFunction(
    () =>
      document.querySelector('[data-team-fingerprint]')?.getAttribute('data-team-fingerprint') === '479147' &&
      !document.getElementById('games-container')?.hasAttribute('aria-busy')
  )
  const rows = await page.$$eval('[data-game-checkbox][data-game-group="upcoming-games"]', inputs =>
    inputs.map(input => `${input.value}:${input.checked ? 'on' : 'off'}`).join(',')
  )
  if (rows !== '7003:on,7001:on') fail(`Campfire Rovers rows are ${rows}`)
  if ((await count()) !== '2 games selected') fail(`selected count for the new team set is ${await count()}`)

  const campfireOnly = page.locator('[data-game-checkbox][data-game-group="upcoming-games"][value="7003"]')
  await campfireOnly.focus()
  await page.keyboard.press('Space')
  if ((await count()) !== '1 game selected') fail(`selected count after deselecting 7003 is ${await count()}`)
  const stored = await page.evaluate(() => window.sessionStorage.getItem('portfolio:soccer:selection:479147'))
  if (JSON.stringify(JSON.parse(stored || 'null')) !== JSON.stringify({ version: 2, deselected: ['7003'] })) fail(`stored selection is ${stored}`)
  return { rows, stored, count: await count() }
}
