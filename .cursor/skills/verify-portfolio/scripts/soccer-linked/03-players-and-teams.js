// Player choice decides the teams offered: Taylor alone plays for Campfire
// Rovers, and both players add Pond Mint United. Checkboxes use the keyboard.
async page => {
  const fail = message => {
    throw new Error(`soccer linked proof (players and teams): ${message}`)
  }
  const teams = () =>
    page.$$eval('#soccer-team-select-form input[name="team_ids"]', inputs =>
      inputs.map(input => ({ id: input.value, name: input.getAttribute('aria-label'), checked: input.checked }))
    )
  const continueToTeams = async () => {
    const [response] = await Promise.all([
      page.waitForResponse(candidate => new URL(candidate.url()).pathname === '/soccer/discover-teams'),
      page.locator('#soccer-player-select-form button[type="submit"]').click(),
    ])
    if (response.status() !== 200) fail(`team discovery answered ${response.status()}`)
    await page.waitForFunction(() => !document.getElementById('soccer-team-stage-content')?.hasAttribute('aria-busy'))
    await page.locator('#soccer-team-select-form').waitFor({ state: 'visible' })
  }

  const craig = page.locator('#soccer-player-select-form input[name="player_ids"][value="1669080"]')
  await craig.focus()
  await page.keyboard.press('Space')
  if (await craig.isChecked()) fail('Space did not deselect Craig')
  await continueToTeams()
  const taylorTeams = await teams()
  if (taylorTeams.map(team => team.name).join('|') !== 'Campfire Rovers') fail(`teams for Taylor are ${JSON.stringify(taylorTeams)}`)

  await craig.focus()
  await page.keyboard.press('Space')
  await continueToTeams()
  const bothTeams = await teams()
  if (bothTeams.map(team => `${team.name}:${team.checked}`).join('|') !== 'Pond Mint United:true|Campfire Rovers:true') {
    fail(`teams for both players are ${JSON.stringify(bothTeams)}`)
  }
  return { taylorTeams, bothTeams }
}
