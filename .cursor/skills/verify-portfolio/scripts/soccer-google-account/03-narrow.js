// The connected Google card fits a narrow phone viewport without horizontal
// scrolling and keeps the connected account readable.
async page => {
  const fail = message => {
    throw new Error(`soccer Google account proof (narrow layout): ${message}`)
  }
  await page.setViewportSize({ width: 390, height: 844 })
  await page.locator('#soccer-google-connection [data-google-account]').scrollIntoViewIfNeeded()
  const layout = await page.evaluate(() => {
    const account = document.querySelector('#soccer-google-connection [data-google-account]').getBoundingClientRect()
    return {
      viewport: window.innerWidth,
      document: document.documentElement.scrollWidth,
      accountLeft: Math.round(account.left),
      accountRight: Math.round(account.right),
    }
  })
  if (layout.document > layout.viewport) fail(`page scrolls horizontally: ${layout.document}px in a ${layout.viewport}px viewport`)
  if (layout.accountLeft < 0 || layout.accountRight > layout.viewport) fail(`connected account is clipped: ${JSON.stringify(layout)}`)
  return layout
}
