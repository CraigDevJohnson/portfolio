// Refetching the same team set in another order keeps every team's color:
// the LPS color, the Team ID fallback, and the shared game's two halves.
async page => {
  const fail = message => {
    throw new Error(`soccer team colors proof (refetch): ${message}`)
  }
  const colors = () =>
    page.$$eval('[data-game-group="upcoming-games"] .soccer-match-row', rows =>
      Object.fromEntries(
        rows.map(row => [
          row.querySelector('[data-game-checkbox]').value,
          `${row.dataset.homeColor}/${row.dataset.awayColor}${row.hasAttribute('data-shared-match') ? ' shared' : ''} ${getComputedStyle(row).backgroundImage}`,
        ])
      )
    )
  const before = await colors()
  // Mark the rendered rows so the comparison reads only the refetched ones.
  await page.$$eval('[data-game-group="upcoming-games"] .soccer-match-row', rows => rows.forEach(row => row.setAttribute('data-proof-before-refetch', '')))

  await page.locator('#team_codes').fill('479147 479691')
  const fetched = page.waitForResponse(response => response.url().endsWith('/soccer/fetch'))
  await page.locator('#fetch-form button[type="submit"]').click()
  if ((await fetched).status() !== 200) fail('the refetch did not succeed')
  await page.waitForFunction(
    () =>
      !document.querySelector('[data-proof-before-refetch]') &&
      !document.getElementById('games-container')?.hasAttribute('aria-busy') &&
      document.querySelector('[data-game-checkbox][data-game-group="upcoming-games"][value="7001"]')
  )

  const after = await colors()
  for (const game of ['7001', '7002', '7003']) {
    if (!after[game]) fail(`game ${game} is missing after the refetch`)
    if (after[game] !== before[game]) fail(`game ${game} changed from ${before[game]} to ${after[game]}`)
  }
  return { '7001': after['7001'].split(' linear')[0], '7002': after['7002'].split(' linear')[0], '7003': after['7003'].split(' linear')[0] }
}
