// Choice-first entry: only the output choice is visible until the visitor
// picks one, and the keyboard choice of an .ics file reveals the public steps.
async page => {
  const fail = message => {
    throw new Error(`soccer planner proof (choose .ics): ${message}`)
  }
  const visible = selector => page.locator(selector).first().isVisible()

  if (new URL(page.url()).pathname !== '/soccer') fail(`route is ${page.url()}`)
  if (!(await visible('[data-soccer-stage="calendar-output"]'))) fail('the output choice is not visible')
  for (const selector of ['#soccer-connections', '[data-soccer-stage="source"]', '[data-soccer-stage="review"]']) {
    if (await visible(selector)) fail(`${selector} is visible before an output is chosen`)
  }
  if ((await page.locator('[data-soccer-output-option]:checked').count()) !== 0) fail('an output is preselected')
  if (!(await page.locator('input[name="calendar_output"][value="google"]').isDisabled())) {
    fail('Google output is choosable although the preview has no Google configuration')
  }

  const ics = page.locator('input[name="calendar_output"][value="ics"]')
  await ics.focus()
  await page.keyboard.press('Space')
  if (!(await ics.isChecked())) fail('Space did not choose the .ics output')

  for (const selector of ['#soccer-connections', '[data-soccer-stage="source"]', '#team_codes', '.soccer-source-option[data-soccer-linked-source]']) {
    if (!(await visible(selector))) fail(`${selector} stayed hidden after choosing .ics`)
  }
  if (await page.locator('[data-soccer-private-stage]:visible').count()) fail('private stages are visible without imported access')
  if (await visible('[data-soccer-stage="review"]')) fail('the empty review stage is visible before a fetch')
  const legend = await page.locator('.soccer-stage-legend li:visible strong').allTextContents()
  if (legend.join('|') !== 'Choose output|Choose source|Review & output') fail(`legend steps are ${legend.join(', ')}`)
  const reviewNumber = (await page.locator('.soccer-stage-legend [data-soccer-review-number]').textContent())?.trim()
  if (reviewNumber !== '3') fail(`review step number is ${reviewNumber}`)
  return { output: 'ics', legend, reviewNumber }
}
