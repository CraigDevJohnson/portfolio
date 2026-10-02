// Craig's button lists his proven team seasons newest LPS season first and
// opens the current one: its scored record and label, the collection status,
// and the completed games newest first from the team's side. Each former
// season then opens with the notice for its collection state.
async page => {
  const fail = message => {
    throw new Error(`soccer team history proof (Craig's seasons): ${message}`)
  }
  const squash = text => (text || '').replace(/\s+/g, ' ').trim()
  const section = page.locator('#soccer-history')
  const opened = key => page.locator(`[data-soccer-team-history-detail="${key}"]`).waitFor({ state: 'visible' })
  const notices = async () => (await section.locator('.soccer-team-history-notice .ui-feedback-title').allTextContents()).map(squash)

  await section.locator('#soccer-team-history-player-1669080').click()
  await opened('479691/169')
  if ((await section.locator('#soccer-team-history-player-1669080').getAttribute('aria-pressed')) !== 'true') fail('Craig is not the pressed player')
  const seasons = await section.locator('[data-soccer-team-history-season]').evaluateAll(buttons =>
    buttons.map(button => `${button.dataset.soccerTeamHistorySeason} ${button.dataset.current === 'true' ? 'current' : 'former'}${button.getAttribute('aria-current') === 'true' ? ' open' : ''}`)
  )
  const wantSeasons = ['479691/169 current open', '479801/168 former', '479802/167 former', '479800/166 former']
  if (seasons.join('|') !== wantSeasons.join('|')) fail(`seasons are ${JSON.stringify(seasons)}`)

  const record = squash(await section.locator('[data-soccer-team-history-record]').textContent())
  for (const want of ['1 W · 1 L · 1 D', '1 win, 1 loss, 1 draw', 'from 3 scored games', 'Calculated from numeric game scores; not official standings.', '1 completed game not counted.']) {
    if (!record.includes(want)) fail(`record ${JSON.stringify(record)} lacks ${JSON.stringify(want)}`)
  }
  const collected = squash(await section.locator('[data-soccer-team-history-collected]').textContent())
  if (!/^Collected \w{3} \d\d\/\d\d\/\d\d \d\d:\d\d [AP]M M[DS]T · LPS returned 6 games for this season$/.test(collected)) fail(`collection status is ${JSON.stringify(collected)}`)
  if ((await notices()).length !== 0) fail(`the current season shows notices ${JSON.stringify(await notices())}`)
  const games = await section.locator('[data-soccer-team-history-game]').evaluateAll(rows =>
    rows.map(row => [
      row.dataset.soccerTeamHistoryGame,
      row.querySelector('.soccer-team-history-opponent').textContent.trim(),
      row.querySelector('.soccer-team-history-score').textContent.trim(),
      row.querySelector('.soccer-team-history-result').textContent.trim(),
    ].join(' | '))
  )
  const wantGames = [
    '8101 | vs Night Mulberry FC | 4–2 | Win',
    '8102 | at Campfire Rovers | 2–2 | Draw',
    '8103 | vs Candle Oat Wanderers | 0–1 | Loss',
    '8104 | vs Rosehip Athletic | postponed | Not counted',
  ]
  if (games.join('|') !== wantGames.join('|')) fail(`games are ${JSON.stringify(games)}`)

  const states = {}
  for (const [key, want] of [
    ['479801/168', ['No games this season']],
    ['479802/167', ['Latest refresh failed']],
    ['479800/166', ['LPS no longer accepts this team']],
  ]) {
    const [team, season] = key.split('/')
    await section.locator(`#soccer-team-history-season-${team}-${season}`).click()
    await opened(key)
    const got = await notices()
    if (got.join('|') !== want.join('|')) fail(`${key} notices are ${JSON.stringify(got)}, want ${JSON.stringify(want)}`)
    states[key] = got
  }
  return { seasons, record, collected, games, states }
}
