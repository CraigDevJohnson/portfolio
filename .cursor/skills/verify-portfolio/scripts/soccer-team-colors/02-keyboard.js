// The keyboard reaches the shared game's selection control over its painted
// halves: Tab moves focus there from select-all with a visible focus ring, and
// Space deselects and reselects it while the live count follows and the row
// keeps both team colors.
async page => {
  const fail = message => {
    throw new Error(`soccer team colors proof (keyboard): ${message}`)
  }
  const shared = page.locator('[data-game-checkbox][data-game-group="upcoming-games"][value="7001"]')
  const count = async () => (await page.locator('[data-selected-count][data-game-group="upcoming-games"]').textContent())?.trim()
  const checkedCount = () => page.locator('[data-game-checkbox][data-game-group="upcoming-games"]:checked').count()
  const paints = () =>
    page.locator('.soccer-match-row:has([data-game-checkbox][value="7001"])').evaluate(row => getComputedStyle(row).backgroundImage)

  await page.locator('[data-select-all][data-game-group="upcoming-games"]').focus()
  let presses = 0
  while (!(await shared.evaluate(input => input === document.activeElement))) {
    if (presses === 12) fail('Tab never reached the shared game from select-all')
    await page.keyboard.press('Tab')
    presses += 1
  }
  const focus = await shared.evaluate(input => {
    const style = getComputedStyle(input)
    const target = input.closest('.game-checkbox-target').getBoundingClientRect()
    return {
      focusVisible: input.matches(':focus-visible'),
      outlineStyle: style.outlineStyle,
      outlineWidth: parseFloat(style.outlineWidth),
      target: { width: Math.round(target.width), height: Math.round(target.height) },
    }
  })
  if (!focus.focusVisible || focus.outlineStyle === 'none' || focus.outlineWidth < 2) fail(`the shared game's control has no visible focus ring: ${JSON.stringify(focus)}`)
  if (focus.target.width < 44 || focus.target.height < 44) fail(`the shared game's control target is ${focus.target.width}x${focus.target.height}px`)

  const before = { selected: await checkedCount(), count: await count(), paints: await paints() }
  const label = selected => `${selected} ${selected === 1 ? 'game' : 'games'} selected`
  if (before.count !== label(before.selected)) fail(`count reads ${before.count} with ${before.selected} selected`)

  await page.keyboard.press('Space')
  if (await shared.isChecked()) fail('Space did not deselect the shared game')
  const deselected = { selected: await checkedCount(), count: await count(), paints: await paints() }
  if (deselected.selected !== before.selected - 1 || deselected.count !== label(before.selected - 1)) {
    fail(`after deselecting, ${deselected.selected} selected and the count reads ${deselected.count}`)
  }
  if (deselected.paints !== before.paints) fail('deselecting repainted the shared game')

  await page.keyboard.press('Space')
  if (!(await shared.isChecked())) fail('Space did not reselect the shared game')
  const reselected = { selected: await checkedCount(), count: await count() }
  if (reselected.selected !== before.selected || reselected.count !== before.count) fail(`after reselecting, the count reads ${reselected.count}`)
  return { presses, focus, before: { selected: before.selected, count: before.count }, deselected: deselected.count, reselected: reselected.count }
}
