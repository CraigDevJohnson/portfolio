// Keyboard deselection of the shared game updates the live count and the
// indeterminate select-all control, and is remembered for the team set.
async page => {
  const fail = message => {
    throw new Error(`soccer planner proof (deselect): ${message}`)
  }
  const shared = page.locator('[data-game-checkbox][data-game-group="upcoming-games"][value="7001"]')
  await shared.focus()
  await page.keyboard.press('Space')
  if (await shared.isChecked()) fail('Space did not deselect game 7001')

  const count = (await page.locator('[data-selected-count][data-game-group="upcoming-games"]').textContent())?.trim()
  if (count !== '2 games selected') fail(`selected count is ${count}`)
  const selectAll = await page
    .locator('[data-select-all][data-game-group="upcoming-games"]')
    .evaluate(input => ({ checked: input.checked, indeterminate: input.indeterminate }))
  if (selectAll.checked || !selectAll.indeterminate) fail(`select-all is ${JSON.stringify(selectAll)}, want indeterminate`)
  const stored = await page.evaluate(() => window.sessionStorage.getItem('portfolio:soccer:selection:479147-479691'))
  if (JSON.stringify(JSON.parse(stored || 'null')) !== JSON.stringify({ version: 2, deselected: ['7001'] })) {
    fail(`stored selection is ${stored}`)
  }
  return { count, selectAll, stored }
}
