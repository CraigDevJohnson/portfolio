// Taylor's current season has no game played yet. Robin has no proven
// season. LPS cannot confirm Jordan's current teams, so only his stored
// season is listed, and it awaits its first collection.
async page => {
  const fail = message => {
    throw new Error(`soccer team history proof (other players): ${message}`)
  }
  const squash = text => (text || '').replace(/\s+/g, ' ').trim()
  const section = page.locator('#soccer-history')
  const notices = async () => (await section.locator('.soccer-team-history-notice .ui-feedback-title').allTextContents()).map(squash)
  const choose = async id => {
    await section.locator(`#soccer-team-history-player-${id}`).click()
    await section.locator(`#soccer-team-history-player-${id}[aria-pressed="true"]`).waitFor()
  }
  const seasons = () => section.locator('[data-soccer-team-history-season]').evaluateAll(buttons => buttons.map(button => button.dataset.soccerTeamHistorySeason))

  await choose(1669081)
  await page.locator('[data-soccer-team-history-detail="479147/170"]').waitFor({ state: 'visible' })
  const taylor = { seasons: await seasons(), notices: await notices(), games: await section.locator('[data-soccer-team-history-game]').count() }
  if (taylor.seasons.join('|') !== '479147/170' || taylor.notices.join('|') !== 'No completed games yet' || taylor.games !== 0) fail(`Taylor shows ${JSON.stringify(taylor)}`)

  await choose(1669082)
  const robin = { seasons: await seasons(), notices: await notices(), details: await section.locator('[data-soccer-team-history-detail]').count() }
  if (robin.seasons.length !== 0 || robin.notices.join('|') !== 'No team seasons yet' || robin.details !== 0) fail(`Robin shows ${JSON.stringify(robin)}`)

  await choose(1669083)
  await page.locator('[data-soccer-team-history-detail="479803/165"]').waitFor({ state: 'visible' })
  const jordan = { seasons: await seasons(), notices: await notices(), text: squash(await section.textContent()) }
  if (jordan.seasons.join('|') !== '479803/165' || jordan.notices.join('|') !== 'Current teams not confirmed|Not collected yet') fail(`Jordan shows ${JSON.stringify(jordan)}`)
  if (!jordan.text.includes('The next refresh is due')) fail('Jordan\'s season does not say when it is next refreshed')
  return { taylor, robin, jordan: { seasons: jordan.seasons, notices: jordan.notices } }
}
