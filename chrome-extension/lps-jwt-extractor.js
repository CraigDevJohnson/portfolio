const CAPTURE_KEY = 'lpsJwtCapture'
const CAPTURE_URLS = [
  'https://letsplaysoccer.com/*',
  'https://www.letsplaysoccer.com/*',
  'https://*.lps-test.com/*',
]
const JWT_PATTERN = /^[A-Za-z0-9-_]+\.[A-Za-z0-9-_]+\.[A-Za-z0-9-_]+$/
const BADGE_COLOR = '#146356'

function logStorageWarning(action, error) {
  console.warn(`Unable to ${action} LPS JWT capture.`, error)
}

function syncBadge(hasCapture) {
  chrome.action.setBadgeText({ text: hasCapture ? 'JWT' : '' })
  if (hasCapture) {
    chrome.action.setBadgeBackgroundColor({ color: BADGE_COLOR })
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

async function saveCapture(token, sourceType, details) {
  const capture = {
    token,
    importValue: `Bearer ${token}`,
    sourceType,
    sourceUrl: details.url,
    capturedAt: new Date().toISOString(),
  }

  try {
    await chrome.storage.local.set({ [CAPTURE_KEY]: capture })
  } catch (error) {
    logStorageWarning('save', error)
    return
  }

  syncBadge(true)
}

chrome.runtime.onInstalled.addListener(() => {
  syncBadge(false)
})

chrome.storage.local.get(CAPTURE_KEY)
  .then(result => {
    syncBadge(Boolean(result[CAPTURE_KEY]?.importValue))
  })
  .catch(error => {
    logStorageWarning('read', error)
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
          void saveCapture(token, 'authorization', details)
          return
        }
      }

      if (name === 'cookie') {
        const token = extractTokenFromCookie(value)
        if (token) {
          void saveCapture(token, 'cookie', details)
          return
        }
      }
    }
  },
  { urls: CAPTURE_URLS },
  ['requestHeaders', 'extraHeaders']
)
