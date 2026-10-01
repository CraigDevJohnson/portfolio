// The linked planner fits a narrow phone viewport without horizontal scroll.
async page => {
  const fail = message => {
    throw new Error(`soccer linked proof (narrow layout): ${message}`)
  }
  await page.setViewportSize({ width: 390, height: 844 })
  const layout = await page.evaluate(() => ({ viewport: window.innerWidth, document: document.documentElement.scrollWidth }))
  if (layout.document > layout.viewport) fail(`page scrolls horizontally: ${layout.document}px in a ${layout.viewport}px viewport`)
  for (const selector of ['#soccer-player-select-form button[type="submit"]', '#soccer-team-select-form button[type="submit"]', '#download-button']) {
    if (!(await page.locator(selector).isVisible())) fail(`${selector} is not visible at 390px`)
  }
  return layout
}
