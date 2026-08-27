const CAPTURE_KEY = 'lpsJwtCapture'
const AUTOFILL_URL = 'https://craigdevjohnson.com/soccer?extension_autofill=1'
const COPY_STATUS_RESET_DELAY_MS = 2000

const statusNode = document.getElementById('status')
const jwtField = document.getElementById('jwt-value')
const sourceTypeNode = document.getElementById('source-type')
const capturedAtNode = document.getElementById('captured-at')
const sourceUrlNode = document.getElementById('source-url')
const copyButton = document.getElementById('copy-btn')
const openButton = document.getElementById('open-btn')
const clearButton = document.getElementById('clear-btn')
let copyStatusResetId = 0

function formatTimestamp(value) {
  if (!value) {
    return '-'
  }

  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return value
  }

  return date.toLocaleString()
}

function setButtonState(hasCapture) {
  copyButton.disabled = !hasCapture
  clearButton.disabled = !hasCapture
}

function renderCapture(capture) {
  if (!capture || !capture.importValue) {
    statusNode.textContent = 'No JWT captured yet.'
    jwtField.value = ''
    sourceTypeNode.textContent = '-'
    capturedAtNode.textContent = '-'
    sourceUrlNode.textContent = 'Visit letsplaysoccer.com while signed in and trigger an authenticated request.'
    setButtonState(false)
    return
  }

  statusNode.textContent = 'JWT captured and ready to use.'
  jwtField.value = capture.importValue
  sourceTypeNode.textContent = capture.sourceType || '-'
  capturedAtNode.textContent = formatTimestamp(capture.capturedAt)
  sourceUrlNode.textContent = capture.sourceUrl || ''
  setButtonState(true)
}

async function loadCapture() {
  const result = await chrome.storage.local.get(CAPTURE_KEY)
  renderCapture(result[CAPTURE_KEY])
}

copyButton.addEventListener('click', async () => {
  if (!jwtField.value) {
    return
  }

  try {
    await navigator.clipboard.writeText(jwtField.value)
    statusNode.textContent = 'Import value copied.'
    window.clearTimeout(copyStatusResetId)
    copyStatusResetId = window.setTimeout(() => {
      loadCapture()
    }, COPY_STATUS_RESET_DELAY_MS)
  } catch {
    statusNode.textContent = 'Unable to copy the import value. Copy it manually from the field.'
  }
})

openButton.addEventListener('click', () => {
  chrome.tabs.create({ url: AUTOFILL_URL })
})

clearButton.addEventListener('click', async () => {
  await chrome.storage.local.remove(CAPTURE_KEY)
  await chrome.action.setBadgeText({ text: '' })
  renderCapture(null)
})

chrome.storage.onChanged.addListener((changes, areaName) => {
  if (areaName !== 'local' || !changes[CAPTURE_KEY]) {
    return
  }

  renderCapture(changes[CAPTURE_KEY].newValue)
})

loadCapture()
