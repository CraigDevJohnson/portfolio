// Open one preview identity's Soccer page (the proof script replaces
// __FIXTURE__), choose the .ics output so Connections appear, and require what
// a visitor in that soccer grant state can see and use. Team IDs stays public.
async page => {
  const fixture = '__FIXTURE__'
  const want = {
    'soccer-signed-out': { notice: 'Sign in with an invited account', signInLinks: 1, account: '', granted: false },
    'soccer-ungranted': { notice: 'has not been granted', signInLinks: 0, account: 'invited.visitor@example.com', granted: false },
    'soccer-granted': { notice: '', signInLinks: 0, account: 'invited.visitor@example.com', granted: true },
  }[fixture]
  const fail = message => {
    throw new Error(`soccer access proof (${fixture}): ${message}`)
  }
  if (!want) fail('unknown fixture')

  const { origin } = new URL(page.url())
  await page.goto(`${origin}/__preview/account/${fixture}`, { waitUntil: 'domcontentloaded' })
  await page.evaluate(() => window.sessionStorage.clear())
  await page.locator('input[name="calendar_output"][value="ics"]').check()
  await page.locator('#soccer-connections').waitFor({ state: 'visible' })

  const squash = text => (text || '').replace(/\s+/g, ' ').trim()
  const notices = page.locator('#soccer-connections .soccer-workspace-feedback', { hasText: 'Private Soccer access' })
  const account = page.locator('nav[aria-label="Main navigation"] .site-account-email')
  const state = {
    route: new URL(page.url()).pathname,
    notice: (await notices.count()) ? squash(await notices.first().textContent()) : '',
    signInLinks: await page.locator('#soccer-connections a[href="/sign-in?return_to=%2Fsoccer"]:visible').count(),
    account: (await account.count()) ? squash(await account.first().textContent()) : '',
    lpsCard: squash(await page.locator('#soccer-lps-connection').textContent()),
    googleCard: squash(await page.locator('#soccer-google-connection').textContent()),
    importButtons: await page.locator('#soccer-lps-connection button[data-open-login-modal]:visible').count(),
    googleConnect: await page.locator('#soccer-google-connection :is(a, button):visible', { hasText: 'Connect Google Calendar' }).count(),
    googleConnectInert: await page.locator('#soccer-google-connection button[disabled]:visible', { hasText: 'Connect Google Calendar' }).count(),
    googleOption: squash(await page.locator('label.soccer-output-option', { has: page.locator('input[value="google"]') }).textContent()),
    teamIDs: await page.locator('#team_codes').isVisible(),
  }

  if (state.route !== `/__preview/account/${fixture}`) fail(`route is ${state.route}`)
  if (want.notice ? !state.notice.includes(want.notice) : state.notice) fail(`notice is ${JSON.stringify(state.notice)}`)
  if (state.signInLinks !== want.signInLinks) fail(`Soccer sign-in links = ${state.signInLinks}`)
  if (state.account !== want.account) fail(`navigation account is ${JSON.stringify(state.account)}`)
  const cardsNeedAccess = [state.lpsCard, state.googleCard].map(text => text.includes('Needs Soccer access'))
  if (cardsNeedAccess.some(needs => needs === want.granted)) fail(`connection cards are ${JSON.stringify([state.lpsCard, state.googleCard])}`)
  if (want.granted) {
    if (state.importButtons !== 1 || state.googleConnect !== 1) fail(`granted controls: import ${state.importButtons}, Google ${state.googleConnect}`)
    if (state.googleConnectInert !== 1) fail('the preview Google connect control is not disabled')
  } else {
    if (state.importButtons || state.googleConnect) fail('private import or Google controls are offered')
    if (!state.googleOption.includes('Needs Soccer access')) fail(`Google output option is ${JSON.stringify(state.googleOption)}`)
  }
  if (!state.teamIDs) fail('Team IDs is hidden')
  return state
}
