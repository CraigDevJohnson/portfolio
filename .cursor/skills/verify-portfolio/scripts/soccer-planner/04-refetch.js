// Refetching the same team set in another order keeps the explicit
// deselection, and the game LPS published since the first fetch starts selected.
async page => {
  const fail = message => {
    throw new Error(`soccer planner proof (refetch): ${message}`)
  }
  await page.locator('#team_codes').fill('479147 479691')
  await page.locator('#fetch-form button[type="submit"]').click()
  await page.waitForFunction(
    () =>
      document.querySelectorAll('[data-game-checkbox][data-game-group="upcoming-games"]').length === 4 &&
      !document.getElementById('games-container')?.hasAttribute('aria-busy')
  )

  const rows = await page.$$eval('[data-game-checkbox][data-game-group="upcoming-games"]', inputs =>
    inputs.map(input => ({ id: input.value, checked: input.checked }))
  )
  const ids = rows.map(row => row.id).join(',')
  if (ids !== '7003,7001,7002,7004') fail(`rows are ${ids}, want 7003,7001,7002,7004`)
  const checked = Object.fromEntries(rows.map(row => [row.id, row.checked]))
  if (checked['7001']) fail('the explicit deselection of 7001 did not survive the refetch')
  if (!checked['7004']) fail('newly published game 7004 did not begin selected')
  const count = (await page.locator('[data-selected-count][data-game-group="upcoming-games"]').textContent())?.trim()
  if (count !== '3 games selected') fail(`selected count is ${count}`)
  const indeterminate = await page.locator('[data-select-all][data-game-group="upcoming-games"]').evaluate(input => input.indeterminate)
  if (!indeterminate) fail('select-all is not indeterminate with one game deselected')
  return { rows, count }
}
