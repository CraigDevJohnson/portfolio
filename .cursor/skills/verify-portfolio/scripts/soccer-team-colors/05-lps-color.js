// Adding Rosehip Athletic to the example teams shows an LPS color beside their
// fallbacks. The fake LPS names Rosehip Athletic's color "  kelly GREEN ", as
// LPS might, which must reach its half of shared game 7002 as green, never as
// the raw text. Green is not a fallback either example team took, so both
// keep their colors: Pond Mint United orange and Campfire Rovers purple.
async page => {
  const fail = message => {
    throw new Error(`soccer team colors proof (LPS color): ${message}`)
  }
  await page.setViewportSize({ width: 1440, height: 1000 })
  // Mark the rendered rows so the checks read only the new fetch.
  await page.$$eval('[data-game-group="upcoming-games"] .soccer-match-row', rows => rows.forEach(row => row.setAttribute('data-proof-before-lps-fetch', '')))

  await page.locator('#team_codes').fill('479691, 479147, 479800')
  const fetched = page.waitForResponse(response => response.url().endsWith('/soccer/fetch'))
  await page.locator('#fetch-form button[type="submit"]').click()
  if ((await fetched).status() !== 200) fail('the fetch with Rosehip Athletic did not succeed')
  await page.waitForFunction(
    () =>
      !document.querySelector('[data-proof-before-lps-fetch]') &&
      !document.getElementById('games-container')?.hasAttribute('aria-busy') &&
      document.querySelector('[data-game-checkbox][data-game-group="upcoming-games"][value="7002"]')
  )

  const rendered = await page.$$eval('[data-game-group="upcoming-games"] .soccer-match-row', rows =>
    rows.map(row => {
      const stops = getComputedStyle(row).backgroundImage.match(/rgba?\([^)]+\)/g) || []
      return {
        id: row.querySelector('[data-game-checkbox]').value,
        colors: `${row.dataset.homeColor}/${row.dataset.awayColor}`,
        shared: row.hasAttribute('data-shared-match'),
        homePaint: stops[0],
        awayPaint: stops[stops.length - 1],
        text: row.textContent.replace(/\s+/g, ' ').trim(),
        inlineStyle: row.getAttribute('style'),
      }
    })
  )
  const row = id => {
    const matches = rendered.filter(candidate => candidate.id === id)
    if (matches.length !== 1) fail(`game ${id} rendered ${matches.length} rows, want one`)
    return matches[0]
  }
  const rosehipShared = row('7002')
  const exampleShared = row('7001')
  const campfireOnly = row('7003')
  if (!rosehipShared.shared || rosehipShared.colors !== 'green/orange' || !rosehipShared.homePaint || rosehipShared.homePaint === rosehipShared.awayPaint) {
    fail(`Rosehip Athletic's game 7002 is ${JSON.stringify(rosehipShared)}, want shared green/orange with two painted halves`)
  }
  if (!rosehipShared.text.includes('Rosehip Athletic vs Pond Mint United')) fail(`game 7002 reads ${rosehipShared.text}`)
  if (!exampleShared.shared || exampleShared.colors !== 'orange/purple' || exampleShared.homePaint === exampleShared.awayPaint) {
    fail(`the example teams' game 7001 is ${JSON.stringify(exampleShared)}, want shared orange/purple`)
  }
  if (campfireOnly.shared || campfireOnly.colors !== 'purple/purple') fail(`Campfire Rovers game 7003 is ${campfireOnly.colors}`)
  // Pond Mint United is painted the same beside Rosehip Athletic as beside Campfire Rovers.
  if (rosehipShared.awayPaint !== exampleShared.homePaint) fail(`Pond Mint United paints ${rosehipShared.awayPaint} in 7002 but ${exampleShared.homePaint} in 7001`)
  for (const candidate of rendered) {
    if (candidate.inlineStyle) fail(`game ${candidate.id} carries an inline style ${candidate.inlineStyle}`)
  }
  const body = await page.locator('#games-container').innerHTML()
  if (/kelly|GREEN/.test(body)) fail('the raw LPS color text reached the page')
  return rendered.map(({ text, inlineStyle, ...rest }) => rest)
}
