// From the production /soccer entry, a signed-out browser's direct requests
// to every Google Calendar route, including the account chooser, are refused
// and uncacheable; nothing reaches Google.
async page => {
  const fail = message => {
    throw new Error(`soccer Google account proof (direct requests): ${message}`)
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
      return { method, path, status: response.status, cacheControl: response.headers.get('Cache-Control'), location: response.headers.get('Location') }
    }
    return Promise.all([
      send('GET', '/soccer/google/connect'),
      send('GET', '/soccer/google/connect?account=choose'),
      send('POST', '/soccer/google/calendar', { calendar_id: 'primary' }),
      send('POST', '/soccer/google/disconnect', {}),
    ])
  })
  for (const result of results) {
    if (result.status !== 401) fail(`${result.method} ${result.path} returned ${result.status}`)
    if (result.cacheControl !== 'no-store') fail(`${result.path} refusal is cacheable: ${result.cacheControl}`)
    if (result.location) fail(`${result.path} redirected to ${result.location}`)
  }
  return results
}
