// The open import dialog, with its history notice, fits a narrow phone
// viewport without horizontal scrolling.
async page => {
  const fail = message => {
    throw new Error(`soccer history notice proof (narrow layout): ${message}`)
  }
  await page.setViewportSize({ width: 390, height: 844 })
  const notice = page.locator('#soccer-history-notice')
  await notice.scrollIntoViewIfNeeded()
  if (!(await notice.isVisible())) fail('the history notice is not visible at 390px')
  const layout = await page.evaluate(() => {
    const box = document.querySelector('#soccer-history-notice').getBoundingClientRect()
    const dialog = document.querySelector('.soccer-login-dialog').getBoundingClientRect()
    return {
      viewport: window.innerWidth,
      document: document.documentElement.scrollWidth,
      noticeLeft: Math.round(box.left),
      noticeRight: Math.round(box.right),
      dialogLeft: Math.round(dialog.left),
      dialogRight: Math.round(dialog.right),
    }
  })
  if (layout.document > layout.viewport) fail(`page scrolls horizontally: ${layout.document}px in a ${layout.viewport}px viewport`)
  if (layout.noticeLeft < 0 || layout.noticeRight > layout.viewport || layout.dialogLeft < 0 || layout.dialogRight > layout.viewport) fail(`notice or dialog is clipped: ${JSON.stringify(layout)}`)
  return layout
}
