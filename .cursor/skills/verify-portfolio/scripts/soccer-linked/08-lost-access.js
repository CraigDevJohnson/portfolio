// LPS withdraws access after an import: the preview LPS accepts a token
// signed "revoked" for the import and refuses its team lookups. Choosing
// players then clears the import, closes the private stages, and explains
// the recovery in the LPS card, which stays visible, without the players.
async page => {
  const fail = message => {
    throw new Error(`soccer linked proof (lost access): ${message}`)
  }
  const squash = text => (text || '').replace(/\s+/g, ' ').trim()
  const encode = value => btoa(JSON.stringify(value)).replace(/=+$/, '').replace(/\+/g, '-').replace(/\//g, '_')
  const revokedJWT = [encode({ alg: 'none', typ: 'JWT' }), encode({ exp: Math.floor(Date.now() / 1000) + 3600 }), 'revoked'].join('.')
  await page.setViewportSize({ width: 1440, height: 1000 })

  await page.locator('#soccer-lps-connection button', { hasText: 'Clear import' }).click()
  await page.locator('#soccer-lps-connection[data-connection-state="disconnected"]').waitFor()
  await page.locator('#soccer-lps-connection button[data-open-login-modal]').click()
  await page.locator('#soccer-import-jwt').fill(revokedJWT)
  await page.locator('#soccer-login-form button[type="submit"]').click()
  await page.locator('#soccer-player-select-form input[name="player_ids"]').first().waitFor({ state: 'visible' })
  await page.locator('#soccer-login-modal').waitFor({ state: 'hidden' })

  const [response] = await Promise.all([
    page.waitForResponse(candidate => new URL(candidate.url()).pathname === '/soccer/discover-teams'),
    page.locator('#soccer-player-select-form button[type="submit"]').click(),
  ])
  const trigger = response.headers()['hx-trigger'] || ''
  if (!trigger.includes('soccer-workflow-reset')) fail(`the rejected token did not close the workflow; HX-Trigger was ${trigger}`)
  const card = page.locator('#soccer-lps-connection')
  await card.locator('.ui-feedback').waitFor({ state: 'visible' })

  const notice = squash(await card.locator('.ui-feedback').textContent())
  if (!notice.includes("Your imported Let's Play Soccer token was rejected.")) fail(`LPS card notice is ${JSON.stringify(notice)}`)
  if ((await card.locator('.ui-feedback').getAttribute('role')) !== 'alert') fail('the LPS card notice is not announced as an alert')
  if (!(await card.locator('button[data-open-login-modal]', { hasText: 'Import fresh access' }).isVisible())) fail('Import fresh access is not visible')
  if (!(await card.locator('a[href="#team_codes"]', { hasText: 'Use manual team IDs' }).isVisible())) fail('the manual Team ID path is not visible')
  if (await page.locator('[data-soccer-private-stage]:visible').count()) fail('private stages stayed visible after LPS rejected the token')
  if (await page.locator('input[name="player_ids"]').count()) fail('linked players remain on the page')
  if (await page.getByText('Taylor Alexandra Johnson-Summit').count()) fail('a linked player name remains on the page')
  const source = squash(await page.locator('#soccer-linked-source').textContent())
  if (!source.includes('Set up LPS access')) fail(`linked source is ${JSON.stringify(source)}`)
  const cookies = await page.context().cookies(`${new URL(page.url()).origin}/soccer`)
  if (cookies.some(cookie => cookie.name === 'lps_session' || cookie.name === 'lps_import_guard')) fail('the rejected import stayed in the browser')
  if (!(await page.locator('#team_codes').isVisible())) fail('manual Team IDs are hidden')

  await page.setViewportSize({ width: 390, height: 844 })
  const layout = await page.evaluate(() => ({ viewport: window.innerWidth, document: document.documentElement.scrollWidth }))
  if (layout.document > layout.viewport) fail(`the recovery scrolls horizontally: ${layout.document}px in a ${layout.viewport}px viewport`)
  await page.setViewportSize({ width: 1440, height: 1000 })
  return { trigger, notice, source, layout }
}
