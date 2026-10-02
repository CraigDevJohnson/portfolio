// At 390px Team history fits without horizontal scroll, and each game stacks
// its kickoff above its opponent and score.
async page => {
  const fail = message => {
    throw new Error(`soccer team history proof (narrow layout): ${message}`)
  }
  await page.setViewportSize({ width: 390, height: 844 })
  await page.locator('#soccer-team-history-player-1669080').click()
  await page.locator('[data-soccer-team-history-detail="479691/169"]').waitFor({ state: 'visible' })
  const layout = await page.evaluate(() => {
    const section = document.getElementById('soccer-history')
    return { viewport: window.innerWidth, document: document.documentElement.scrollWidth, section: section.scrollWidth, sectionBox: section.clientWidth }
  })
  if (layout.document > layout.viewport) fail(`page scrolls horizontally: ${layout.document}px in a ${layout.viewport}px viewport`)
  if (layout.section > layout.sectionBox) fail(`Team history overflows: ${layout.section}px in ${layout.sectionBox}px`)
  const stacked = await page.locator('[data-soccer-team-history-game]').evaluateAll(rows =>
    rows.every(row => row.querySelector('time').getBoundingClientRect().bottom <= row.querySelector('.soccer-team-history-opponent').getBoundingClientRect().top + 1)
  )
  if (!stacked) fail('a game does not stack its kickoff above its opponent at 390px')
  return { ...layout, stacked }
}
