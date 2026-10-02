// Clear import ends the import, and the response hides Team history again
// with the LPS card back to Not imported.
async page => {
  const fail = message => {
    throw new Error(`soccer team history proof (clear import): ${message}`)
  }
  const squash = text => (text || '').replace(/\s+/g, ' ').trim()
  await page.locator('#soccer-lps-connection').getByRole('button', { name: 'Clear import' }).click()
  const section = page.locator('#soccer-history')
  await section.waitFor({ state: 'hidden' })
  const lpsCard = squash(await page.locator('#soccer-lps-connection').textContent())
  if (!lpsCard.includes('Not imported')) fail(`LPS card is ${JSON.stringify(lpsCard)}`)
  if ((await section.locator('*').count()) !== 0) fail('the hidden section keeps team history')
  if ((await page.locator('a[href="#soccer-history"]').count()) !== 0) fail('the page still links to Team history')
  return { lpsCard: 'Not imported', hidden: true }
}
