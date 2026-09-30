// The production /soccer entry wires no durable archive: it shows no history
// notice, and a signed-out browser's direct import that claims the disclosure
// is refused while the public Team ID lookup still answers.
async page => {
  const fail = message => {
    throw new Error(`soccer history notice proof (production entry): ${message}`)
  }
  const { origin } = new URL(page.url())
  await page.goto(`${origin}/soccer`, { waitUntil: 'domcontentloaded' })
  const html = await page.evaluate(() => document.documentElement.outerHTML)
  if (html.includes('soccer-history-notice') || html.includes('name="history_notice"')) fail('production page shows history collection')
  const results = await page.evaluate(async () => {
    const send = async form => {
      const response = await fetch(form.team_codes ? '/soccer/fetch' : '/soccer/import', {
        method: 'POST',
        credentials: 'same-origin',
        redirect: 'manual',
        body: new URLSearchParams(form),
        headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      })
      const text = await response.text()
      return { status: response.status, cacheControl: response.headers.get('Cache-Control'), rows: (text.match(/data-game-checkbox/g) || []).length, body: text.slice(0, 160) }
    }
    return {
      disclosedImport: await send({ jwt: 'not-a-jwt', history_notice: 'indefinite' }),
      teamIDs: await send({ team_codes: '479147' }),
    }
  })
  if (results.disclosedImport.status !== 401 || results.disclosedImport.cacheControl !== 'no-store') fail(`direct disclosed import returned ${results.disclosedImport.status} (${results.disclosedImport.cacheControl}): ${results.disclosedImport.body}`)
  if (results.teamIDs.status !== 200 || results.teamIDs.rows === 0) fail(`public Team ID fetch returned ${results.teamIDs.status} with ${results.teamIDs.rows} game rows`)
  return results
}
