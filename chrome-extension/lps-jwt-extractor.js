const CAPTURE_KEY = 'lpsJwtCapture'
const CAPTURE_URLS = [
  'https://letsplaysoccer.com/*',
  'https://www.letsplaysoccer.com/*',
  'https://*.lps-test.com/*',
]
const JWT_PATTERN = /^[A-Za-z0-9-_]+\.[A-Za-z0-9-_]+\.[A-Za-z0-9-_]+$/

function syncBadge(hasCapture) {
  chrome.action.setBadgeText({ text: hasCapture ? 'JWT' : '' })
  if (hasCapture) {
    chrome.action.setBadgeBackgroundColor({ color: '#146356' })
  }
}

function extractTokenFromAuthorization(value) {
  if (!value) {
    return ''
  }

  const trimmed = value.trim()
  if (trimmed.toLowerCase().startsWith('bearer ')) {
    const token = trimmed.slice(7).trim()
    return JWT_PATTERN.test(token) ? token : ''
  }

  return JWT_PATTERN.test(trimmed) ? trimmed : ''
}

function extractTokenFromCookie(value) {
  if (!value) {
    return ''
  }

  const parts = value.split(';')
  for (const part of parts) {
    const [name, ...rest] = part.trim().split('=')
    if (name !== 'lps_token') {
      continue
    }

    const token = rest.join('=').trim()
    return JWT_PATTERN.test(token) ? token : ''
  }

  return ''
}

function saveCapture(token, sourceType, details) {
  const capture = {
    token,
    importValue: `Bearer ${token}`,
    sourceType,
    sourceUrl: details.url,
    capturedAt: new Date().toISOString(),
  }

  chrome.storage.local.set({ [CAPTURE_KEY]: capture })
  syncBadge(true)
  console.info('Captured LPS JWT from', sourceType, details.url)
}

chrome.runtime.onInstalled.addListener(() => {
  syncBadge(false)
})

chrome.storage.local.get(CAPTURE_KEY).then(result => {
  syncBadge(Boolean(result[CAPTURE_KEY]?.importValue))
})

chrome.webRequest.onBeforeSendHeaders.addListener(
  details => {
    if (!Array.isArray(details.requestHeaders)) {
      return
    }

    for (const header of details.requestHeaders) {
      const name = header.name.toLowerCase()
      const value = header.value || ''

      if (name === 'authorization') {
        const token = extractTokenFromAuthorization(value)
        if (token) {
          saveCapture(token, 'authorization', details)
          return
        }
      }

      if (name === 'cookie') {
        const token = extractTokenFromCookie(value)
        if (token) {
          saveCapture(token, 'cookie', details)
          return
        }
      }
    }
  },
  { urls: CAPTURE_URLS },
  ['requestHeaders', 'extraHeaders']
)
