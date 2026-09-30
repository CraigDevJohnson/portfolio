// In forced-colors mode the browser replaces the team paints with system
// colors, so each row must still read by its text alone: team names, the
// neutral score, and a selection control the keyboard can toggle.
async page => {
  const fail = message => {
    throw new Error(`soccer team colors proof (forced colors): ${message}`)
  }
  await page.emulateMedia({ forcedColors: 'active' })
  try {
    if (!(await page.evaluate(() => window.matchMedia('(forced-colors: active)').matches))) fail('forced colors did not activate')
    const rows = await page.$$eval('.soccer-match-row', rows =>
      rows
        .filter(row => row.offsetParent !== null)
        .map(row => {
          const names = [...row.querySelectorAll('.soccer-matchup strong, .soccer-matchup span')].map(name => ({
            text: name.textContent.trim(),
            color: getComputedStyle(name).color,
          }))
          const style = getComputedStyle(row)
          return {
            id: row.querySelector('[data-game-checkbox]').value,
            names,
            background: style.backgroundColor,
            text: row.textContent.replace(/\s+/g, ' ').trim(),
          }
        })
    )
    const shared = rows.find(row => row.id === 'preview-past-2') || fail('the shared past result is not visible')
    if (!shared.text.includes('Pond Mint United vs Campfire Rovers') || !shared.text.includes('Home 4 – Away 2')) fail(`the shared past result reads ${shared.text}`)
    for (const row of rows) {
      for (const name of row.names) {
        if (name.color === row.background) fail(`${row.id} name ${name.text} matches its row background ${row.background}`)
      }
    }
    const control = page.locator('[data-game-checkbox][value="preview-past-2"]')
    await control.focus()
    await page.keyboard.press('Space')
    const toggled = !(await control.isChecked())
    await page.keyboard.press('Space')
    if (!toggled || !(await control.isChecked())) fail('Space did not toggle the shared past result in forced colors')
    return rows.map(({ id, background, names }) => ({ id, background, names: names.map(name => `${name.text}: ${name.color}`) }))
  } finally {
    await page.emulateMedia({ forcedColors: 'none' })
  }
}
