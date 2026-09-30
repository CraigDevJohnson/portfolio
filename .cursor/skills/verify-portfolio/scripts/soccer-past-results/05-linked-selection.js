// On the linked source, deselecting the newest past result is remembered
// across a refetch of the same teams.
async page => {
  const fail = message => {
    throw new Error(`soccer past results proof (linked selection): ${message}`)
  }
  const count = async () => (await page.locator('[data-selected-count][data-game-group="past-results"]').textContent())?.trim()
  const selection = () =>
    page.$$eval('[data-game-checkbox][data-game-group="past-results"]', inputs =>
      inputs.map(input => `${input.value}:${input.checked ? 'on' : 'off'}`).join(',')
    )

  const newest = page.locator('[data-game-checkbox][data-game-group="past-results"][value="7000"]')
  await newest.focus()
  await page.keyboard.press('Space')
  if (await newest.isChecked()) fail('Space did not deselect past result 7000')
  if ((await count()) !== '2 games selected') fail(`count after deselection is ${await count()}`)
  if (!(await page.locator('[data-select-all][data-game-group="past-results"]').evaluate(input => input.indeterminate))) fail('past select-all is not indeterminate')

  const [response] = await Promise.all([
    page.waitForResponse(candidate => new URL(candidate.url()).pathname === '/soccer/fetch'),
    page.locator('#soccer-team-select-form button[type="submit"]').click(),
  ])
  if (response.status() !== 200) fail(`the linked refetch answered ${response.status()}`)
  await page.waitForFunction(() => !document.getElementById('games-container')?.hasAttribute('aria-busy'))
  await page.locator('[data-game-checkbox][data-game-group="past-results"][value="7000"]').waitFor({ state: 'visible' })
  const refetched = await selection()
  if (refetched !== '7000:off,6998:on,6990:on') fail(`past results after refetch are ${refetched}`)
  if ((await count()) !== '2 games selected') fail(`count after refetch is ${await count()}`)
  return { refetched, count: await count() }
}
