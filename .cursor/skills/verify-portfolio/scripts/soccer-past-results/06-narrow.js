// The Google-mode past-result review fits a narrow phone viewport without
// horizontal scroll, with its selection controls reachable.
async page => {
  const fail = message => {
    throw new Error(`soccer past results proof (narrow layout): ${message}`)
  }
  await page.setViewportSize({ width: 390, height: 844 })
  const layout = await page.evaluate(() => ({ viewport: window.innerWidth, document: document.documentElement.scrollWidth }))
  if (layout.document > layout.viewport) fail(`page scrolls horizontally: ${layout.document}px in a ${layout.viewport}px viewport`)
  for (const selector of ['#past-results-heading', '[data-select-all][data-game-group="past-results"]', '[data-game-checkbox][data-game-group="past-results"][value="6990"]']) {
    const target = page.locator(selector)
    await target.scrollIntoViewIfNeeded()
    if (!(await target.isVisible())) fail(`${selector} is not visible at 390px`)
  }
  return layout
}
