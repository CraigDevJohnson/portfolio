// The Sync report and the past results copy fit a narrow phone viewport
// without horizontal scrolling or clipping.
async page => {
  const fail = message => {
    throw new Error(`soccer result sync proof (narrow layout): ${message}`)
  }
  await page.setViewportSize({ width: 390, height: 844 })
  await page.locator('#past-results-feedback').scrollIntoViewIfNeeded()
  const layout = await page.evaluate(() => {
    const box = selector => {
      const rect = document.querySelector(selector).getBoundingClientRect()
      return { left: Math.round(rect.left), right: Math.round(rect.right) }
    }
    return {
      viewport: window.innerWidth,
      document: document.documentElement.scrollWidth,
      report: box('#past-results-feedback .ui-feedback-message'),
      copy: box('#past-results-form .games-section-copy'),
    }
  })
  if (layout.document > layout.viewport) fail(`page scrolls horizontally: ${layout.document}px in a ${layout.viewport}px viewport`)
  for (const [name, rect] of Object.entries({ report: layout.report, copy: layout.copy })) {
    if (rect.left < 0 || rect.right > layout.viewport) fail(`${name} is clipped: ${JSON.stringify(layout)}`)
  }
  return layout
}
