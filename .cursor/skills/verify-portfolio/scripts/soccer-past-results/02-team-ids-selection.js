// Past-result selection with the keyboard: deselecting one result updates the
// count and select-all, select-all clears and restores the whole group, and a
// refetch of the same team set in another order keeps the explicit
// deselection.
async page => {
  const fail = message => {
    throw new Error(`soccer past results proof (Team ID selection): ${message}`)
  }
  const count = async () => (await page.locator('[data-selected-count][data-game-group="past-results"]').textContent())?.trim()
  const selection = () =>
    page.$$eval('[data-game-checkbox][data-game-group="past-results"]', inputs =>
      inputs.map(input => `${input.value}:${input.checked ? 'on' : 'off'}`).join(',')
    )
  const selectAll = page.locator('[data-select-all][data-game-group="past-results"]')
  const middle = page.locator('[data-game-checkbox][data-game-group="past-results"][value="6998"]')

  await middle.focus()
  await page.keyboard.press('Space')
  if (await middle.isChecked()) fail('Space did not deselect past result 6998')
  if ((await count()) !== '2 games selected') fail(`count after deselection is ${await count()}`)
  if (!(await selectAll.evaluate(input => input.indeterminate))) fail('past select-all is not indeterminate')
  const stored = await page.evaluate(() => window.sessionStorage.getItem('portfolio:soccer:selection:479147-479691'))
  if (!JSON.parse(stored || 'null')?.deselected?.includes('6998')) fail(`stored selection is ${stored}`)

  // An indeterminate select-all is unchecked, so Space selects the group,
  // then clears it, then selects it again.
  await selectAll.focus()
  await page.keyboard.press('Space')
  const all = await selection()
  if (all !== '7000:on,6998:on,6990:on' || (await count()) !== '3 games selected') fail(`select-all from a partial selection left ${all}, ${await count()}`)
  await page.keyboard.press('Space')
  const cleared = await selection()
  if (cleared !== '7000:off,6998:off,6990:off' || (await count()) !== '0 games selected') fail(`select-all cleared ${cleared}, ${await count()}`)
  await page.keyboard.press('Space')
  const restored = await selection()
  if (restored !== '7000:on,6998:on,6990:on' || (await count()) !== '3 games selected') fail(`select-all restored ${restored}, ${await count()}`)
  await middle.focus()
  await page.keyboard.press('Space')

  const [response] = await Promise.all([
    page.waitForResponse(candidate => new URL(candidate.url()).pathname === '/soccer/fetch'),
    (async () => {
      await page.locator('#team_codes').fill('479147 479691')
      await page.locator('#team_codes').press('Enter')
    })(),
  ])
  if (response.status() !== 200) fail(`the refetch answered ${response.status()}`)
  await page.waitForFunction(() => !document.getElementById('games-container')?.hasAttribute('aria-busy'))
  await page.locator('[data-game-checkbox][data-game-group="past-results"][value="6998"]').waitFor({ state: 'visible' })
  const refetched = await selection()
  if (refetched !== '7000:on,6998:off,6990:on') fail(`past results after refetch are ${refetched}`)
  if ((await count()) !== '2 games selected') fail(`count after refetch is ${await count()}`)
  return { stored, all, cleared, restored, refetched, count: await count() }
}
