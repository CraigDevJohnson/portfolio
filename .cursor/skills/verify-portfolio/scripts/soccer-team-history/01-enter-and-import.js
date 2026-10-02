// Open the preview's history account, which stands in for site Cognito on a
// server whose history store is a fixture, and choose the .ics output. Before
// an import the Team history section is an empty, hidden placeholder. An
// import through the dialog reveals it with every linked player, none chosen,
// and the LPS card links to it. The token is a fake the preview LPS accepts.
async page => {
  const fail = message => {
    throw new Error(`soccer team history proof (enter and import): ${message}`)
  }
  const squash = text => (text || '').replace(/\s+/g, ' ').trim()
  const encode = value => btoa(JSON.stringify(value)).replace(/=+$/, '').replace(/\+/g, '-').replace(/\//g, '_')
  const fakeJWT = [encode({ alg: 'none', typ: 'JWT' }), encode({ exp: Math.floor(Date.now() / 1000) + 3600 }), 'preview-signature'].join('.')

  const { origin } = new URL(page.url())
  await page.goto(`${origin}/__preview/account/soccer-history`, { waitUntil: 'domcontentloaded' })
  await page.evaluate(() => window.sessionStorage.clear())
  await page.reload({ waitUntil: 'domcontentloaded' })
  if (new URL(page.url()).pathname !== '/soccer') fail(`the preview entry landed on ${page.url()}`)
  const section = page.locator('#soccer-history')
  if ((await section.count()) !== 1) fail('the page has no Team history placeholder')
  if ((await section.isVisible()) || (await section.locator('*').count()) !== 0) fail('Team history shows before an import')

  await page.locator('input[name="calendar_output"][value="ics"]').check()
  await page.locator('#soccer-lps-connection button[data-open-login-modal]').click()
  const dialog = page.locator('#soccer-login-modal')
  await dialog.waitFor({ state: 'visible' })
  await page.locator('#soccer-import-jwt').fill(fakeJWT)
  await page.locator('#soccer-login-form button[type="submit"]').click()
  await section.waitFor({ state: 'visible' })
  await dialog.waitFor({ state: 'hidden' })

  const players = await section.locator('[data-soccer-team-history-player]').evaluateAll(buttons =>
    buttons.map(button => `${button.dataset.soccerTeamHistoryPlayer} ${button.textContent.trim()} ${button.getAttribute('aria-pressed')}`)
  )
  const want = ['1669080 Craig Johnson false', '1669081 Taylor Alexandra Johnson-Summit false', '1669082 Robin Johnson false', '1669083 Jordan Johnson false']
  if (players.join('|') !== want.join('|')) fail(`players are ${JSON.stringify(players)}`)
  const hint = squash(await section.locator('.soccer-team-history-hint').textContent())
  if (hint !== 'Choose a player to load their team seasons.') fail(`hint is ${JSON.stringify(hint)}`)
  if ((await section.locator('[data-soccer-team-history-season], [data-soccer-team-history-detail]').count()) !== 0) fail('seasons load before a player is chosen')
  const link = page.locator('#soccer-lps-connection a[href="#soccer-history"]')
  if ((await link.count()) !== 1 || squash(await link.textContent()) !== 'View team history') fail('the LPS card does not link to Team history')
  return { players, hint, link: 'View team history' }
}
