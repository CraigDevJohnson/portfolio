// At a 390px phone width the colored rows fit without horizontal scrolling:
// the shared game still shows both halves, every cell and team name stays
// inside its row unclipped, and the keyboard still toggles its selection.
async page => {
  const fail = message => {
    throw new Error(`soccer team colors proof (narrow layout): ${message}`)
  }
  await page.setViewportSize({ width: 390, height: 844 })
  await page.locator('.soccer-match-row:has([data-game-checkbox][value="7001"])').scrollIntoViewIfNeeded()

  const layout = await page.evaluate(() => {
    const rows = [...document.querySelectorAll('[data-game-group="upcoming-games"] .soccer-match-row')]
    return {
      viewport: window.innerWidth,
      document: document.documentElement.scrollWidth,
      rows: rows.map(row => {
        const box = row.getBoundingClientRect()
        const stops = getComputedStyle(row).backgroundImage.match(/rgba?\([^)]+\)/g) || []
        const target = row.querySelector('.game-checkbox-target').getBoundingClientRect()
        return {
          id: row.querySelector('[data-game-checkbox]').value,
          halves: [stops[0], stops[stops.length - 1]],
          shared: row.hasAttribute('data-shared-match'),
          overflowingCells: [...row.querySelectorAll('.soccer-match-select, .soccer-match-detail')]
            .filter(cell => {
              const cellBox = cell.getBoundingClientRect()
              return cellBox.left < box.left - 1 || cellBox.right > box.right + 1
            })
            .map(cell => cell.dataset.label || 'selection'),
          clippedNames: [...row.querySelectorAll('.soccer-matchup strong, .soccer-matchup span')]
            .filter(name => name.scrollWidth > name.clientWidth + 1 || name.getBoundingClientRect().right > box.right + 1)
            .map(name => name.textContent.trim()),
          target: { width: Math.round(target.width), height: Math.round(target.height) },
        }
      }),
    }
  })
  if (layout.document > layout.viewport) fail(`page scrolls horizontally: ${layout.document}px in a ${layout.viewport}px viewport`)
  for (const row of layout.rows) {
    if (row.overflowingCells.length) fail(`game ${row.id} cells leave the row: ${row.overflowingCells.join(', ')}`)
    if (row.clippedNames.length) fail(`game ${row.id} clips team names: ${row.clippedNames.join(', ')}`)
    if (row.target.width < 44 || row.target.height < 44) fail(`game ${row.id} selection target is ${row.target.width}x${row.target.height}px`)
  }
  const shared = layout.rows.find(row => row.id === '7001')
  if (!shared?.shared || shared.halves[0] === shared.halves[1]) fail(`the shared game lost a half at 390px: ${JSON.stringify(shared)}`)

  const control = page.locator('[data-game-checkbox][data-game-group="upcoming-games"][value="7001"]')
  await control.focus()
  await page.keyboard.press('Space')
  const deselected = !(await control.isChecked())
  await page.keyboard.press('Space')
  if (!deselected || !(await control.isChecked())) fail('Space did not toggle the shared game at 390px')
  return layout
}
