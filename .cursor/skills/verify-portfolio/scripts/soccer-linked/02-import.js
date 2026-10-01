// Import LPS access through the real dialog. The token is a fake the preview
// LPS accepts, never a real JWT. The import reveals the player and team
// stages, and the review becomes step 5.
async page => {
  const fail = message => {
    throw new Error(`soccer linked proof (import): ${message}`)
  }
  const squash = text => (text || '').replace(/\s+/g, ' ').trim()
  const encode = value => btoa(JSON.stringify(value)).replace(/=+$/, '').replace(/\+/g, '-').replace(/\//g, '_')
  const fakeJWT = [encode({ alg: 'none', typ: 'JWT' }), encode({ exp: Math.floor(Date.now() / 1000) + 3600 }), 'preview-signature'].join('.')

  await page.locator('#soccer-lps-connection button[data-open-login-modal]').click()
  const dialog = page.locator('#soccer-login-modal')
  await dialog.waitFor({ state: 'visible' })
  await page.locator('#soccer-import-jwt').fill(fakeJWT)
  await page.locator('#soccer-login-form button[type="submit"]').click()
  await page.locator('#soccer-player-select-form input[name="player_ids"]').first().waitFor({ state: 'visible' })
  await dialog.waitFor({ state: 'hidden' })

  const lpsCard = squash(await page.locator('#soccer-lps-connection').textContent())
  if (!lpsCard.includes('Imported in this browser')) fail(`LPS card is ${JSON.stringify(lpsCard)}`)
  const players = await page.$$eval('#soccer-player-select-form label.soccer-player-option', labels =>
    labels.map(label => label.textContent.replace(/\s+/g, ' ').trim())
  )
  if (players.join('|') !== 'Craig Johnson Primary player|Taylor Alexandra Johnson-Summit') fail(`players are ${players.join(', ')}`)
  const legend = await page.locator('.soccer-stage-legend li:visible strong').allTextContents()
  if (legend.join('|') !== 'Choose output|Choose source|Select players|Confirm teams|Review & output') fail(`legend steps are ${legend.join(', ')}`)
  const reviewNumber = (await page.locator('.soccer-stage-legend [data-soccer-review-number]').textContent())?.trim()
  if (reviewNumber !== '5') fail(`review step number is ${reviewNumber}`)
  for (const selector of ['[data-soccer-stage="players"]', '[data-soccer-stage="teams"]']) {
    if (!(await page.locator(selector).isVisible())) fail(`${selector} stayed hidden after the import`)
  }
  const source = squash(await page.locator('#soccer-linked-source').textContent())
  if (!source.includes('2 linked player(s) are ready')) fail(`linked source after the import is ${JSON.stringify(source)}`)
  return { lpsCard, players, legend, reviewNumber, source }
}
