// From the production /soccer entry, choose .ics with the keyboard and fetch
// both preview teams from the fake LPS. Pond Mint United's LPS color
// ("  kelly GREEN ") must reach its rows as green, Campfire Rovers (no LPS
// color) must keep its Team ID fallback, and their shared game must be one row
// painted half in each, with every label keeping WCAG AA text contrast over
// both painted halves.
async page => {
  const fail = message => {
    throw new Error(`soccer team colors proof (fetch): ${message}`)
  }

  if (new URL(page.url()).pathname !== '/soccer') fail(`route is ${page.url()}`)
  const ics = page.locator('input[name="calendar_output"][value="ics"]')
  await ics.focus()
  await page.keyboard.press('Space')
  if (!(await ics.isChecked())) fail('Space did not choose the .ics output')

  await page.locator('#team_codes').fill('479691, 479147')
  await page.locator('#team_codes').press('Enter')
  await page.locator('[data-game-checkbox][data-game-group="upcoming-games"][value="7001"]').waitFor()
  await page.waitForFunction(() => !document.getElementById('games-container')?.hasAttribute('aria-busy'))

  // Reads what the browser rendered for each row: its color names, whether it
  // is a shared match, the painted halves of its computed gradient, and the
  // text contrast of each label over each half.
  const rendered = await page.$$eval('[data-game-group="upcoming-games"] .soccer-match-row', rows => {
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
    // The surface under an element over one painted half: the paint, then
    // every translucent layer between the row and the element.
    const surface = (element, row, paint) => {
      const layers = []
      for (let node = element; node && node !== row; node = node.parentElement) layers.unshift(node)
      return layers.reduce((below, node) => over(parse(getComputedStyle(node).backgroundColor), below), paint)
    }
    const key = color => `rgb(${Math.round(color.r)}, ${Math.round(color.g)}, ${Math.round(color.b)})`
    return rows.map(row => {
      const stops = [...getComputedStyle(row).backgroundImage.matchAll(/rgba?\([^)]+\)/g)].map(match => parse(match[0]))
      const home = stops[0]
      const away = stops[stops.length - 1]
      const labels = []
      for (const cell of row.querySelectorAll('.soccer-match-detail')) {
        labels.push({ name: `${cell.dataset.label} label`, element: cell, color: parse(getComputedStyle(cell, '::before').color) })
        for (const text of cell.querySelectorAll('strong, span')) {
          if (text.textContent.trim()) labels.push({ name: `${cell.dataset.label}: ${text.textContent.trim()}`, element: text, color: parse(getComputedStyle(text).color) })
        }
      }
      const weakest = labels
        .flatMap(label => [home, away].map(paint => ({ name: label.name, ratio: contrast(label.color, surface(label.element, row, paint)) })))
        .sort((a, b) => a.ratio - b.ratio)[0]
      return {
        id: row.querySelector('[data-game-checkbox]').value,
        homeColor: row.dataset.homeColor,
        awayColor: row.dataset.awayColor,
        shared: row.hasAttribute('data-shared-match'),
        homePaint: key(home),
        awayPaint: key(away),
        stops: stops.length,
        text: row.textContent.replace(/\s+/g, ' ').trim(),
        weakestLabel: weakest.name,
        weakestContrast: Math.round(weakest.ratio * 100) / 100,
        inlineStyle: row.getAttribute('style'),
      }
    })
  })

  const row = id => rendered.find(candidate => candidate.id === id) || fail(`game ${id} is not rendered`)
  const shared = row('7001')
  const pondMintOnly = row('7002')
  const campfireOnly = row('7003')
  if (rendered.filter(candidate => candidate.id === '7001').length !== 1) fail('the shared game is not one row')
  if (!shared.shared || shared.homeColor !== 'green' || shared.awayColor !== 'purple') {
    fail(`shared game 7001 is ${shared.homeColor}/${shared.awayColor} shared=${shared.shared}, want green/purple shared`)
  }
  if (shared.stops < 2 || shared.homePaint === shared.awayPaint) fail(`shared game halves are not two colors: ${shared.homePaint} / ${shared.awayPaint}`)
  if (pondMintOnly.homeColor !== 'green' || pondMintOnly.shared || pondMintOnly.homePaint !== pondMintOnly.awayPaint) {
    fail(`Pond Mint United game 7002 is ${pondMintOnly.homeColor} shared=${pondMintOnly.shared}`)
  }
  if (campfireOnly.homeColor !== 'purple' || campfireOnly.shared || campfireOnly.homePaint !== campfireOnly.awayPaint) {
    fail(`Campfire Rovers game 7003 is ${campfireOnly.homeColor} shared=${campfireOnly.shared}`)
  }
  // Each team is painted the same way wherever it appears.
  if (pondMintOnly.homePaint !== shared.homePaint) fail(`Pond Mint United paints ${pondMintOnly.homePaint} alone but ${shared.homePaint} in the shared game`)
  if (campfireOnly.homePaint !== shared.awayPaint) fail(`Campfire Rovers paints ${campfireOnly.homePaint} alone but ${shared.awayPaint} in the shared game`)
  if (!shared.text.includes('Pond Mint United vs Campfire Rovers')) fail(`shared game text is ${shared.text}`)
  for (const candidate of rendered) {
    if (candidate.inlineStyle) fail(`game ${candidate.id} carries an inline style ${candidate.inlineStyle}`)
    if (candidate.weakestContrast < 4.5) fail(`game ${candidate.id} ${candidate.weakestLabel} has ${candidate.weakestContrast}:1 contrast over a team half`)
  }
  const body = await page.locator('#games-container').innerHTML()
  if (/kelly|GREEN/.test(body)) fail('the raw LPS color text reached the page')
  return rendered.map(({ text, inlineStyle, ...rest }) => rest)
}
