// The planner fits a narrow phone viewport without horizontal scrolling.
async page => {
  const fail = message => {
    throw new Error(`soccer planner proof (narrow layout): ${message}`)
  }
  await page.setViewportSize({ width: 390, height: 844 })
  const layout = await page.evaluate(() => ({
    viewport: window.innerWidth,
    document: document.documentElement.scrollWidth,
    rows: document.querySelectorAll('[data-game-checkbox][data-game-group="upcoming-games"]').length,
  }))
  if (layout.document > layout.viewport) fail(`page scrolls horizontally: ${layout.document}px in a ${layout.viewport}px viewport`)
  for (const selector of ['#team_codes', '#download-button', '[data-select-all][data-game-group="upcoming-games"]']) {
    if (!(await page.locator(selector).isVisible())) fail(`${selector} is not visible at 390px`)
  }
  return layout
}
