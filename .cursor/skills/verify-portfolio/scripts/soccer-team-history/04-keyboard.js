// From the keyboard, Enter on Craig's button loads his seasons and Tab
// reaches his former season; Enter there opens it, and focus stays on the
// season button the swap replaced, which is now the open one.
async page => {
  const fail = message => {
    throw new Error(`soccer team history proof (keyboard): ${message}`)
  }
  const active = () => page.evaluate(() => ({ id: document.activeElement?.id || '', current: document.activeElement?.getAttribute('aria-current') || '' }))
  // htmx wires swapped buttons only once the swap settles, so a key press
  // waits for that, as a person's next key press always would.
  const settled = () =>
    page.waitForFunction(() => !document.querySelector('#soccer-team-history-panel.htmx-settling, #soccer-team-history-panel .htmx-added'))

  await page.locator('#soccer-team-history-player-1669080').focus()
  await page.keyboard.press('Enter')
  await page.locator('[data-soccer-team-history-detail="479691/169"]').waitFor({ state: 'visible' })
  await settled()
  const afterPlayer = await active()
  if (afterPlayer.id !== 'soccer-team-history-player-1669080') fail(`focus left Craig's button for ${JSON.stringify(afterPlayer)}`)

  let tabs = 0
  while ((await active()).id !== 'soccer-team-history-season-479801-168') {
    if (++tabs > 10) fail('Tab never reached the 168 season button')
    await page.keyboard.press('Tab')
  }
  await page.keyboard.press('Enter')
  await page.locator('[data-soccer-team-history-detail="479801/168"]').waitFor({ state: 'visible' })
  await settled()
  const afterSeason = await active()
  if (afterSeason.id !== 'soccer-team-history-season-479801-168' || afterSeason.current !== 'true') fail(`focus after opening the season is ${JSON.stringify(afterSeason)}`)
  return { afterPlayer, tabs, afterSeason }
}
