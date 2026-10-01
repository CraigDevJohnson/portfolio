// Open the preview's granted linked-player account, which stands in for site
// Cognito, and choose the .ics output with the keyboard. Until LPS access is
// imported, the linked-player source asks for it and no private stage shows.
async page => {
  const fail = message => {
    throw new Error(`soccer linked proof (enter and choose .ics): ${message}`)
  }
  const visible = selector => page.locator(selector).first().isVisible()
  const squash = text => (text || '').replace(/\s+/g, ' ').trim()

  const { origin } = new URL(page.url())
  await page.goto(`${origin}/__preview/account/soccer-linked`, { waitUntil: 'domcontentloaded' })
  await page.evaluate(() => window.sessionStorage.clear())
  await page.reload({ waitUntil: 'domcontentloaded' })
  if (new URL(page.url()).pathname !== '/soccer') fail(`the preview entry landed on ${page.url()}`)
  const account = squash(await page.locator('nav[aria-label="Main navigation"] .site-account-email').first().textContent())
  if (account !== 'invited.visitor@example.com') fail(`navigation account is ${JSON.stringify(account)}`)
  for (const selector of ['#soccer-connections', '[data-soccer-stage="source"]', '[data-soccer-stage="review"]']) {
    if (await visible(selector)) fail(`${selector} is visible before an output is chosen`)
  }

  const ics = page.locator('input[name="calendar_output"][value="ics"]')
  await ics.focus()
  await page.keyboard.press('Space')
  if (!(await ics.isChecked())) fail('Space did not choose the .ics output')

  const lpsCard = squash(await page.locator('#soccer-lps-connection').textContent())
  if (!lpsCard.includes('Not imported')) fail(`LPS card is ${JSON.stringify(lpsCard)}`)
  if ((await page.locator('#soccer-lps-connection button[data-open-login-modal]:visible').count()) !== 1) fail('Import access is not offered')
  const source = squash(await page.locator('#soccer-linked-source').textContent())
  if (!source.includes('Use linked players') || !source.includes('Set up LPS access')) fail(`linked source is ${JSON.stringify(source)}`)
  if (await page.locator('[data-soccer-private-stage]:visible').count()) fail('private stages are visible before an import')
  if (!(await visible('#team_codes'))) fail('manual Team IDs are hidden')
  return { route: '/soccer', account, lpsCard, source }
}
