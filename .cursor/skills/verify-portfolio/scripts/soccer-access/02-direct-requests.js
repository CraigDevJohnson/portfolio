// Hiding controls is not the boundary: from the production /soccer page, a
// signed-out browser's direct requests to private routes are refused while
// the public Team ID lookup still answers from the preview fake LPS.
async page => {
  const fail = message => {
    throw new Error(`soccer access proof (direct requests): ${message}`)
  }
  const { origin } = new URL(page.url())
  await page.goto(`${origin}/soccer`, { waitUntil: 'domcontentloaded' })
  const results = await page.evaluate(async () => {
    const send = async (method, path, form) => {
      const init = { method, credentials: 'same-origin', redirect: 'manual' }
      if (form) {
        init.body = new URLSearchParams(form)
        init.headers = { 'Content-Type': 'application/x-www-form-urlencoded' }
      }
      const response = await fetch(path, init)
      const text = await response.text()
      const rows = (text.match(/data-game-checkbox/g) || []).length
      return { method, path, status: response.status, cacheControl: response.headers.get('Cache-Control'), rows, body: text.slice(0, 160) }
    }
    return Promise.all([
      send('POST', '/soccer/import', { jwt: 'not-a-jwt' }),
      send('POST', '/soccer/discover-teams', { player_ids: '1001' }),
      send('POST', '/soccer/fetch', { player_ids: '1001' }),
      send('POST', '/soccer/download', { player_ids: '1001', selected: '7001' }),
      send('GET', '/soccer/google/connect'),
      send('POST', '/soccer/google/add', { team_codes: '479147', selected: '7002' }),
      send('POST', '/soccer/fetch', { team_codes: '479147' }),
    ])
  })
  const publicFetch = results.pop()
  for (const result of results) {
    if (result.status !== 401) fail(`${result.method} ${result.path} returned ${result.status}: ${result.body}`)
    if (result.cacheControl !== 'no-store') fail(`${result.path} refusal is cacheable: ${result.cacheControl}`)
  }
  if (publicFetch.status !== 200 || publicFetch.rows === 0) fail(`public Team ID fetch returned ${publicFetch.status} with ${publicFetch.rows} game rows`)
  const html = await page.evaluate(() => document.body.innerHTML)
  for (const control of ['data-open-login-modal', 'href="/soccer/google/connect"']) {
    if (html.includes(control)) fail(`production page offers ${control} to a signed-out visitor`)
  }
  return results
    .map(({ method, path, status, body }) => `${method} ${path} -> ${status} ${JSON.stringify(body.trim())}`)
    .concat(`POST /soccer/fetch team_codes=479147 -> ${publicFetch.status} with ${publicFetch.rows} game rows`)
}
