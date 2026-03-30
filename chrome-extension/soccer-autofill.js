const CAPTURE_KEY = 'lpsJwtCapture'
const AUTOFILL_QUERY_KEY = 'extension_autofill'
const FIELD_SELECTOR = '#soccer-import-jwt'
const OPEN_BUTTON_SELECTOR = '[data-open-login-modal]'
let cachedImportValue = ''
let applyQueued = false

function dispatchFieldEvents(field) {
  field.dispatchEvent(new Event('input', { bubbles: true }))
  field.dispatchEvent(new Event('change', { bubbles: true }))
}

function fillImportField(importValue) {
  const field = document.querySelector(FIELD_SELECTOR)
  if (!field || !importValue) {
    return false
  }

  if (field.value.trim() === importValue.trim()) {
    return true
  }

  field.value = importValue
  field.dataset.extensionAutofilled = 'true'
  dispatchFieldEvents(field)
  return true
}

function cleanAutofillQueryParam() {
  const url = new URL(window.location.href)
  if (!url.searchParams.has(AUTOFILL_QUERY_KEY)) {
    return
  }

  url.searchParams.delete(AUTOFILL_QUERY_KEY)
  window.history.replaceState({}, '', url)
}

function maybeOpenImportModal() {
  const url = new URL(window.location.href)
  if (url.searchParams.get(AUTOFILL_QUERY_KEY) !== '1') {
    return false
  }

  const button = document.querySelector(OPEN_BUTTON_SELECTOR)
  if (!button) {
    return false
  }

  button.click()
  cleanAutofillQueryParam()
  return true
}

function applyCaptureToPage() {
  if (!cachedImportValue) {
    cleanAutofillQueryParam()
    return
  }

  fillImportField(cachedImportValue)
  maybeOpenImportModal()
}

function scheduleApplyCapture() {
  if (applyQueued) {
    return
  }

  applyQueued = true
  window.requestAnimationFrame(() => {
    applyQueued = false
    applyCaptureToPage()
  })
}

async function refreshCapture() {
  const result = await chrome.storage.local.get(CAPTURE_KEY)
  const capture = result[CAPTURE_KEY]
  cachedImportValue = capture?.importValue || ''
  scheduleApplyCapture()
}

chrome.storage.onChanged.addListener((changes, areaName) => {
  if (areaName !== 'local' || !changes[CAPTURE_KEY]) {
    return
  }

  cachedImportValue = changes[CAPTURE_KEY].newValue?.importValue || ''
  scheduleApplyCapture()
})

const observer = new MutationObserver(() => {
  scheduleApplyCapture()
})

observer.observe(document.documentElement, {
  childList: true,
  subtree: true,
})

refreshCapture()
