// The google-connected fixture renders a scored shared match. In Google mode,
// chosen with the keyboard, its row is half each team's color with a neutral
// home-away score that takes neither side, the Team ID fallback row stays
// one color, and the result text keeps WCAG AA contrast over both halves at
// desktop and 390px widths.
async page => {
  const fail = message => {
    throw new Error(`soccer team colors proof (shared result): ${message}`)
  }
  const { origin } = new URL(page.url())
  await page.setViewportSize({ width: 1440, height: 1000 })
  await page.goto(`${origin}/__preview/soccer/google-connected`, { waitUntil: 'domcontentloaded' })
  await page.evaluate(() => window.sessionStorage.clear())
  await page.reload({ waitUntil: 'domcontentloaded' })

  const google = page.locator('input[name="calendar_output"][value="google"]')
  await google.focus()
  await page.keyboard.press('Space')
  if (!(await google.isChecked())) fail('Space did not choose the Google output')
  await page.locator('#past-results-form').waitFor({ state: 'visible' })

  const read = () =>
    page.$$eval('.soccer-match-row', rows => {
      // Computed colors arrive as rgb(), color(srgb ...), or, for Tailwind
      // palette utilities, oklab()/oklch(); all become 0-255 sRGB with alpha.
      const parse = value => {
        const rgb = value.match(/^rgba?\(([^)]+)\)$/)
        if (rgb) {
          const [r, g, b, a = '1'] = rgb[1].split(/[\s,/]+/).filter(Boolean)
          return { r: +r, g: +g, b: +b, a: +a }
        }
        const srgb = value.match(/^color\(srgb ([-\d.e]+) ([-\d.e]+) ([-\d.e]+)(?: \/ ([\d.]+))?\)$/)
        if (srgb) return { r: srgb[1] * 255, g: srgb[2] * 255, b: srgb[3] * 255, a: srgb[4] === undefined ? 1 : +srgb[4] }
        const ok = value.match(/^okl(ab|ch)\(([-\d.e]+) ([-\d.e]+) ([-\d.e]+)(?: \/ ([\d.]+))?\)$/)
        if (ok) {
          const lightness = +ok[2]
          const [a, b] = ok[1] === 'ab' ? [+ok[3], +ok[4]] : [ok[3] * Math.cos((ok[4] * Math.PI) / 180), ok[3] * Math.sin((ok[4] * Math.PI) / 180)]
          const l = (lightness + 0.3963377774 * a + 0.2158037573 * b) ** 3
          const m = (lightness - 0.1055613458 * a - 0.0721549255 * b) ** 3
          const s = (lightness - 0.0894841775 * a - 1.291485548 * b) ** 3
          const encode = linear => 255 * Math.min(1, Math.max(0, linear <= 0.0031308 ? 12.92 * linear : 1.055 * linear ** (1 / 2.4) - 0.055))
          return {
            r: encode(4.0767416621 * l - 3.3077115913 * m + 0.2309699292 * s),
            g: encode(-1.2684380046 * l + 2.6097574011 * m - 0.3413193965 * s),
            b: encode(-0.0041960863 * l - 0.7034186147 * m + 1.707614701 * s),
            a: ok[5] === undefined ? 1 : +ok[5],
          }
        }
        throw new Error(`unrecognized computed color ${value}`)
      }
      const over = (top, below) => ({
        r: top.a * top.r + (1 - top.a) * below.r,
        g: top.a * top.g + (1 - top.a) * below.g,
        b: top.a * top.b + (1 - top.a) * below.b,
        a: 1,
      })
      const luminance = color => {
        const channel = value => {
          const c = value / 255
          return c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4
        }
        return 0.2126 * channel(color.r) + 0.7152 * channel(color.g) + 0.0722 * channel(color.b)
      }
      const contrast = (a, b) => {
        const [light, dark] = [luminance(a), luminance(b)].sort((x, y) => y - x)
        return (light + 0.05) / (dark + 0.05)
      }
      const surface = (element, row, paint) => {
        const layers = []
        for (let node = element; node && node !== row; node = node.parentElement) layers.unshift(node)
        return layers.reduce((below, node) => over(parse(getComputedStyle(node).backgroundColor), below), paint)
      }
      return rows
        .filter(row => row.offsetParent !== null)
        .map(row => {
          const stops = [...getComputedStyle(row).backgroundImage.matchAll(/rgba?\([^)]+\)/g)].map(match => parse(match[0]))
          const halves = [stops[0], stops[stops.length - 1]]
          const result = row.querySelector('.soccer-match-meta span:not(.season-badge)')
          const resultContrast = result
            ? Math.min(...halves.map(paint => contrast(parse(getComputedStyle(result).color), surface(result, row, paint))))
            : null
          const box = row.getBoundingClientRect()
          return {
            id: row.querySelector('[data-game-checkbox]').value,
            group: row.querySelector('[data-game-checkbox]').dataset.gameGroup,
            colors: `${row.dataset.homeColor}/${row.dataset.awayColor}`,
            shared: row.hasAttribute('data-shared-match'),
            distinctHalves: JSON.stringify(halves[0]) !== JSON.stringify(halves[1]),
            result: result?.textContent.trim() || '',
            resultContrast: resultContrast === null ? null : Math.round(resultContrast * 100) / 100,
            resultInside: result ? result.getBoundingClientRect().right <= box.right + 1 : true,
            text: row.textContent.replace(/\s+/g, ' ').trim(),
          }
        })
    })

  const check = (rows, width) => {
    const row = id => rows.find(candidate => candidate.id === id) || fail(`${id} is not visible at ${width}px`)
    const sharedResult = row('preview-past-2')
    if (sharedResult.group !== 'past-results' || !sharedResult.shared || sharedResult.colors !== 'green/orange' || !sharedResult.distinctHalves) {
      fail(`shared past result at ${width}px is ${JSON.stringify(sharedResult)}`)
    }
    if (sharedResult.result !== 'Home 4 – Away 2' || /Win|Loss|Draw/.test(sharedResult.text)) fail(`shared past result reads ${sharedResult.text}`)
    if (!sharedResult.text.includes('Pond Mint United vs Campfire Rovers')) fail(`shared past result lacks both names: ${sharedResult.text}`)
    const sharedUpcoming = row('preview-upcoming-1')
    if (!sharedUpcoming.shared || sharedUpcoming.colors !== 'green/orange' || !sharedUpcoming.distinctHalves) fail(`shared upcoming game at ${width}px is ${JSON.stringify(sharedUpcoming)}`)
    const fallback = row('preview-upcoming-2')
    if (fallback.shared || fallback.colors !== 'purple/purple' || fallback.distinctHalves) fail(`Team ID fallback row at ${width}px is ${JSON.stringify(fallback)}`)
    for (const candidate of rows) {
      if (candidate.resultContrast !== null && candidate.resultContrast < 4.5) fail(`${candidate.id} result has ${candidate.resultContrast}:1 contrast at ${width}px`)
      if (!candidate.resultInside) fail(`${candidate.id} result leaves its row at ${width}px`)
    }
  }

  const desktop = await read()
  check(desktop, 1440)
  await page.setViewportSize({ width: 390, height: 844 })
  await page.locator('.soccer-match-row:has([value="preview-past-2"])').scrollIntoViewIfNeeded()
  const narrow = await read()
  check(narrow, 390)
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth)
  if (overflow > 0) fail(`the Google-mode schedule scrolls horizontally by ${overflow}px at 390px`)
  return { desktop: desktop.map(({ text, ...rest }) => rest), narrowOverflow: overflow }
}
