// Main JavaScript functionality
;(function () {
  'use strict'

  const HEADER_SCROLL_SHADOW_THRESHOLD = 100
  const MODAL_FOCUSABLE_SELECTOR =
    'button:not([disabled]), input:not([disabled]), a[href], select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'
  const prefersReducedMotionQuery = window.matchMedia('(prefers-reduced-motion: reduce)')
  // Add new page-level stat sections here so their counters animate on reveal.
  const COUNTER_SECTIONS = [
    { section: '.hero-stats', duration: 2000, threshold: 0.1 },
    { section: '.page-stats-section', duration: 2000 },
    { section: '.experience-hero-stats', duration: 1500 },
    { section: '.experience-summary', duration: 1500 },
  ]

  function getOverlayBackgroundElements(root, activeElements) {
    return Array.from(root.children).flatMap(element => {
      if (activeElements.includes(element)) {
        return []
      }

      const nestedActiveElements = activeElements.filter(activeElement => element.contains(activeElement))
      if (nestedActiveElements.length > 0) {
        return getOverlayBackgroundElements(element, nestedActiveElements)
      }

      return [element]
    })
  }

  function setOverlayBackgroundInert(activeElements, previousStates, isInert) {
    if (isInert) {
      getOverlayBackgroundElements(document.body, activeElements).forEach(element => {
        if (!previousStates.has(element)) {
          previousStates.set(element, element.inert)
        }
        element.inert = true
      })
      return
    }

    previousStates.forEach((wasInert, element) => {
      if (element.isConnected) {
        element.inert = wasInert
      }
    })
    previousStates.clear()
  }

  // Mobile menu toggle
  const mobileMenuBtn = document.getElementById('mobile-menu-btn')
  const mobileNav = document.getElementById('mobile-nav')
  const mobileMenuLabel = mobileMenuBtn?.querySelector('[data-menu-label]') || null
  const mobileNavBreakpoint = window.matchMedia('(max-width: 69.999rem)')
  let observer = null

  // Keep portal validation and AWS errors visible while preserving their
  // meaningful HTTP status codes. HTMX does not swap non-2xx responses by
  // default, so the server marks only intentional portal error fragments.
  document.body.addEventListener('htmx:beforeSwap', event => {
    const response = event.detail?.xhr
    if (response?.getResponseHeader('X-Portal-Fragment-Error') !== 'true') {
      return
    }

    event.detail.shouldSwap = true
    event.detail.isError = false
  })

  if (mobileMenuBtn && mobileNav) {
    const mobileNavBackgroundState = new Map()

    const setMobileNavBackgroundInert = isInert =>
      setOverlayBackgroundInert([mobileNav, mobileMenuBtn], mobileNavBackgroundState, isInert)

    const getMobileNavFocusableElements = () =>
      [mobileMenuBtn, ...mobileNav.querySelectorAll(MODAL_FOCUSABLE_SELECTOR)].filter(
        element => !element.hidden && element.getAttribute('aria-hidden') !== 'true'
      )

    const setMobileNavState = isOpen => {
      const nextState = Boolean(isOpen) && mobileNavBreakpoint.matches

      mobileMenuBtn.setAttribute('aria-expanded', String(nextState))
      mobileMenuBtn.setAttribute('aria-label', nextState ? 'Close navigation menu' : 'Open navigation menu')
      mobileNav.classList.toggle('hidden', !nextState)
      mobileNav.classList.toggle('flex', nextState)
      mobileNav.setAttribute('aria-hidden', String(!nextState))
      document.documentElement.classList.toggle('overflow-hidden', nextState)
      document.documentElement.classList.toggle('overscroll-none', nextState)
      document.body.classList.toggle('overflow-hidden', nextState)
      document.body.classList.toggle('overscroll-none', nextState)
      setMobileNavBackgroundInert(nextState)

      if (mobileMenuLabel) {
        mobileMenuLabel.textContent = nextState ? 'Close' : 'Menu'
      }

      if (nextState) {
        mobileNav.scrollTop = 0
      }
    }

    const isMobileNavOpen = () => mobileNav.classList.contains('flex') && !mobileNav.classList.contains('hidden')

    const closeMobileNav = () => {
      setMobileNavState(false)
    }

    mobileMenuBtn.addEventListener('click', () => {
      if (isMobileNavOpen()) {
        closeMobileNav()
        return
      }

      setMobileNavState(true)
      window.requestAnimationFrame(() => {
        if (!isMobileNavOpen()) {
          return
        }

        const currentLink = mobileNav.querySelector('[aria-current="page"]')
        const firstLink = mobileNav.querySelector('.nav-link')
        ;(currentLink || firstLink)?.focus()
      })
    })

    // Close menu when clicking on a link
    mobileNav.querySelectorAll('.nav-link').forEach(link => {
      link.addEventListener('click', closeMobileNav)
    })

    mobileNav.addEventListener('click', event => {
      if (event.target === mobileNav) {
        closeMobileNav()
      }
    })

    // Close menu when clicking outside
    document.addEventListener('click', e => {
      if (isMobileNavOpen() && !mobileMenuBtn.contains(e.target) && !mobileNav.contains(e.target)) {
        closeMobileNav()
      }
    })

    document.addEventListener('keydown', event => {
      if (!isMobileNavOpen()) {
        return
      }

      if (event.key === 'Escape') {
        event.preventDefault()
        closeMobileNav()
        mobileMenuBtn.focus()
        return
      }

      if (event.key !== 'Tab') {
        return
      }

      const focusableElements = getMobileNavFocusableElements()
      if (focusableElements.length === 0) {
        return
      }

      const firstElement = focusableElements[0]
      const lastElement = focusableElements[focusableElements.length - 1]
      const currentIndex = focusableElements.indexOf(document.activeElement)

      if (event.shiftKey && (currentIndex <= 0 || document.activeElement === firstElement)) {
        event.preventDefault()
        lastElement.focus()
      } else if (!event.shiftKey && (currentIndex === -1 || document.activeElement === lastElement)) {
        event.preventDefault()
        firstElement.focus()
      }
    })

    const handleMobileNavBreakpointChange = event => {
      if (!event.matches) {
        closeMobileNav()
      }
    }

    if (typeof mobileNavBreakpoint.addEventListener === 'function') {
      mobileNavBreakpoint.addEventListener('change', handleMobileNavBreakpointChange)
    } else {
      mobileNavBreakpoint.addListener(handleMobileNavBreakpointChange)
    }

    closeMobileNav()
  }

  // Soccer page functionality - using data attributes for flexibility
	const SOCCER_SELECTION_STORAGE_PREFIX = 'portfolio:soccer:selection:'

  function soccerSelectionStorageKey(form) {
    const fingerprint = form?.closest('[data-team-fingerprint]')?.getAttribute('data-team-fingerprint')?.trim()
    return fingerprint ? `${SOCCER_SELECTION_STORAGE_PREFIX}${fingerprint}` : ''
  }

  function pruneSoccerSelectionKeys(currentKey = '') {
    try {
      const staleKeys = []
      for (let index = 0; index < window.sessionStorage.length; index += 1) {
        const key = window.sessionStorage.key(index)
        if (key?.startsWith(SOCCER_SELECTION_STORAGE_PREFIX) && key !== currentKey) {
          staleKeys.push(key)
        }
      }
      staleKeys.forEach(key => window.sessionStorage.removeItem(key))
    } catch (_error) {
      // Selection persistence is an enhancement; storage restrictions must not block the schedule.
    }
  }

  function restoreSoccerSelection(form) {
    const key = soccerSelectionStorageKey(form)
    if (!key) {
      return false
    }
    pruneSoccerSelectionKeys(key)
    try {
			const stored = JSON.parse(window.sessionStorage.getItem(key) || 'null')
			if (
				stored?.version !== 1 ||
				!Array.isArray(stored.upcoming) ||
				!Array.isArray(stored.past) ||
				[...stored.upcoming, ...stored.past].some(gameID => typeof gameID !== 'string')
			) {
				try {
					window.sessionStorage.removeItem(key)
				} catch (_storageError) {
					// Keep server defaults when storage cannot be changed.
				}
        return false
      }
			const selectedGameIDs = new Set([...stored.upcoming, ...stored.past])
      form.querySelectorAll('[data-game-checkbox]').forEach(checkbox => {
        checkbox.checked = selectedGameIDs.has(checkbox.value)
      })
      return true
    } catch (_error) {
			try {
				window.sessionStorage.removeItem(key)
			} catch (_storageError) {
				// Keep server defaults when storage is unavailable.
			}
      return false
    }
  }

  function persistSoccerSelection(form) {
    const key = soccerSelectionStorageKey(form)
    if (!key) {
      return
    }
		const selectedGameIDs = gameGroup =>
			Array.from(
				form.querySelectorAll(`[data-game-checkbox][data-game-group="${gameGroup}"]:checked`),
				checkbox => checkbox.value
			)
		const stored = {
			version: 1,
			upcoming: selectedGameIDs('upcoming-games'),
			past: selectedGameIDs('past-results'),
		}
    try {
      pruneSoccerSelectionKeys(key)
			window.sessionStorage.setItem(key, JSON.stringify(stored))
    } catch (_error) {
      // Ignore unavailable or quota-limited session storage.
    }
  }

  function clearSoccerSelection() {
    pruneSoccerSelectionKeys()
  }

  function setupSoccerSelectAll() {
		document.querySelectorAll('[data-soccer-form][data-team-fingerprint]').forEach(form => {
			if (form.dataset.selectionRestored === 'true') {
				return
			}
			form.dataset.selectionRestored = 'true'
			restoreSoccerSelection(form)
		})

    const sectionSelectAllControls = document.querySelectorAll('[data-select-all][data-game-group]')

    sectionSelectAllControls.forEach(selectAll => {
      if (selectAll.dataset.bound === 'true') {
        return
      }

      const gameGroup = selectAll.dataset.gameGroup
      if (!gameGroup) {
        return
      }

      const getGroupCheckboxes = () =>
        Array.from(document.querySelectorAll(`[data-game-checkbox][data-game-group="${gameGroup}"]`))

      const syncGroupState = () => {
        const gameCheckboxes = getGroupCheckboxes()
        const selectedCount = gameCheckboxes.filter(checkbox => checkbox.checked).length

        if (gameCheckboxes.length === 0) {
          selectAll.checked = false
          selectAll.indeterminate = false
          return
        }

        selectAll.checked = selectedCount === gameCheckboxes.length
        selectAll.indeterminate = selectedCount > 0 && selectedCount < gameCheckboxes.length

        const countLabel = document.querySelector(
          `[data-selected-count][data-game-group="${gameGroup}"]`
        )
        if (countLabel) {
          countLabel.textContent = `${selectedCount} ${selectedCount === 1 ? 'game' : 'games'} selected`
        }

        document.querySelectorAll(`[data-game-action][data-game-group="${gameGroup}"]`).forEach(action => {
          if (action.hasAttribute('data-selection-locked')) {
            action.disabled = true
            action.setAttribute('aria-disabled', 'true')
            return
          }
          if (selectedCount === 0) {
            action.disabled = true
          } else if (action.getAttribute('aria-busy') !== 'true') {
            action.disabled = false
          }
        })
      }

      selectAll.dataset.bound = 'true'

      selectAll.addEventListener('change', function () {
        getGroupCheckboxes().forEach(checkbox => {
          checkbox.checked = selectAll.checked
        })
        syncGroupState()
				persistSoccerSelection(selectAll.closest('[data-soccer-form]'))
      })

      getGroupCheckboxes().forEach(checkbox => {
        if (checkbox.dataset.selectBound === 'true') {
          return
        }

        checkbox.dataset.selectBound = 'true'
		checkbox.addEventListener('change', function () {
			syncGroupState()
			persistSoccerSelection(checkbox.closest('[data-soccer-form]'))
		})
      })

      syncGroupState()
    })
  }

  function animateCounter(counter, duration) {
    const targetYear = counter.dataset.targetYear
    const targetValue = counter.dataset.target
    const suffix = counter.dataset.suffix || ''
    const currentValue = Number.parseInt((counter.textContent || '').replace(/[^\d-]/g, ''), 10) || 0
    const finalValue = targetYear
      ? new Date().getFullYear() - Number.parseInt(targetYear, 10)
      : Number.parseInt(targetValue, 10) || 0

    if (prefersReducedMotionQuery.matches) {
      counter.textContent = `${finalValue}${suffix}`
      return
    }

    const start = performance.now()

    function update(now) {
      const elapsed = now - start
      const progress = Math.min(elapsed / duration, 1)
      const ease = 1 - Math.pow(1 - progress, 3)
      counter.textContent = `${Math.floor(currentValue + (finalValue - currentValue) * ease)}${suffix}`

      if (progress < 1) {
        requestAnimationFrame(update)
        return
      }

      counter.textContent = `${finalValue}${suffix}`
    }

    requestAnimationFrame(update)
  }

  function observeCounterSection(sectionSelector, counterSelector, duration, threshold = 0.3) {
    const section = document.querySelector(sectionSelector)
    if (!section || section.dataset.countersBound === 'true') {
      return
    }

    section.dataset.countersBound = 'true'

    const observer = new IntersectionObserver(
      entries => {
        entries.forEach(entry => {
          if (!entry.isIntersecting) {
            return
          }

          entry.target.querySelectorAll(counterSelector).forEach(counter => {
            animateCounter(counter, duration)
          })
          observer.unobserve(entry.target)
        })
      },
      { threshold }
    )

    observer.observe(section)
  }

  function moveFocusByArrowKey(event, items, currentIndex) {
    if (currentIndex === -1 || items.length === 0) {
      return
    }

    let nextIndex = currentIndex

    switch (event.key) {
      case 'ArrowRight':
      case 'ArrowDown':
        nextIndex = (currentIndex + 1) % items.length
        break
      case 'ArrowLeft':
      case 'ArrowUp':
        nextIndex = (currentIndex - 1 + items.length) % items.length
        break
      case 'Home':
        nextIndex = 0
        break
      case 'End':
        nextIndex = items.length - 1
        break
      default:
        return
    }

    event.preventDefault()
    items[nextIndex].focus()
  }

  let skillsRequestSequence = 0
  let activeSkillsRequest = null
  let skillsControlFocusSnapshot = null
  const skillsKeyboardActivations = new WeakSet()
  const skillsDetailRequestContext = new WeakMap()
  let skillsDetailRequestSequence = 0
  let activeSkillsDetailRequest = null

  function isSkillsFilterRequestElement(element) {
    if (!(element instanceof Element)) {
      return false
    }

    return (
      element.matches('[data-skills-search-form], [data-skills-search-input], [data-skill-filter-category], [data-skill-filter-proficiency], [data-skills-clear], [data-skills-retry]') ||
      element.closest('[data-skills-search-form]') !== null
    )
  }

  function currentSkillsStateURL() {
    return document.getElementById('skills-filter-controls')?.dataset.skillsState || window.location.pathname + window.location.search
  }

  function requestedSkillsURL(event) {
    const path = event.detail?.pathInfo?.finalRequestPath || event.detail?.pathInfo?.requestPath
    if (!path) {
      return currentSkillsStateURL()
    }
    try {
      const url = new URL(path, window.location.href)
      return url.pathname + url.search
    } catch (_error) {
      return path
    }
  }

  function setSkillsBusy(isBusy) {
    const results = document.getElementById('skills-filterable')
    if (results) {
      results.setAttribute('aria-busy', String(Boolean(isBusy)))
    }
  }

  function setSkillsRequestState(state) {
    const workbench = document.querySelector('[data-skills-workbench-history]')
    if (workbench) {
      workbench.dataset.skillsRequestState = state
    }
  }

  function hideSkillsFilterError() {
    const error = document.getElementById('skills-filter-error')
    if (error) {
      error.hidden = true
    }
  }

  function showSkillsFilterError() {
    const error = document.getElementById('skills-filter-error')
    if (error) {
      error.hidden = false
    }
  }

  function snapshotSkillsControlFocus() {
    const active = document.activeElement
    const controls = document.getElementById('skills-filter-controls')
    if (!(active instanceof Element) || !controls?.contains(active)) {
      skillsControlFocusSnapshot = null
      document.querySelector('[data-skills-history-focus]')?.removeAttribute('data-skills-focus-snapshot')
      return
    }

    const snapshot = {
      id: active.id || '',
      search: active.matches('[data-skills-search-input]'),
      selectionStart: null,
      selectionEnd: null,
      selectionDirection: null,
    }
    if (active instanceof HTMLInputElement && typeof active.selectionStart === 'number') {
      snapshot.selectionStart = active.selectionStart
      snapshot.selectionEnd = active.selectionEnd
      snapshot.selectionDirection = active.selectionDirection
    }
    skillsControlFocusSnapshot = snapshot
    const historyFocus = document.querySelector('[data-skills-history-focus]')
    if (historyFocus) {
      historyFocus.setAttribute('data-skills-focus-snapshot', JSON.stringify(snapshot))
    }
  }

  function restoreSkillsControlFocus() {
    const snapshot = skillsControlFocusSnapshot
    skillsControlFocusSnapshot = null
    if (!snapshot) {
      return
    }

    let target = null
    if (snapshot.id) {
      target = document.getElementById(snapshot.id)
    } else if (snapshot.search) {
      target = document.querySelector('[data-skills-search-input]')
    }

    if (target?.closest('[hidden]')) {
      target = document.querySelector('[data-skills-search-input]')
    }

    if (!target?.isConnected) {
      return
    }

    target.focus({ preventScroll: true })
    if (
      target instanceof HTMLInputElement &&
      snapshot.selectionStart !== null &&
      typeof target.setSelectionRange === 'function'
    ) {
      const max = target.value.length
      target.setSelectionRange(
        Math.min(snapshot.selectionStart, max),
        Math.min(snapshot.selectionEnd, max),
        snapshot.selectionDirection || 'none'
      )
    }
  }

  function clearSkillsBindingMarkers(root = document) {
    const marked = []
    if (root instanceof Element && (root.dataset.kbBound || root.dataset.remoteImageBound)) {
      marked.push(root)
    }
    if (typeof root.querySelectorAll === 'function') {
      marked.push(...root.querySelectorAll('[data-kb-bound], [data-remote-image-bound]'))
    }
    marked.forEach(element => {
      delete element.dataset.kbBound
      delete element.dataset.remoteImageBound
    })
  }

  function resetSkillsTransientState() {
    activeSkillsRequest = null
    setSkillsBusy(false)
    setSkillsRequestState('idle')
    hideSkillsFilterError()
  }

  function skillsRequestRecord(event) {
    const xhr = event.detail?.xhr
    if (!xhr) {
      return null
    }
    if (activeSkillsRequest?.xhr === xhr) {
      return activeSkillsRequest
    }
    return null
  }

  function isActiveSkillsRequest(event) {
    return skillsRequestRecord(event) !== null
  }

  function beginSkillsFilterRequest(event) {
    const element = event.detail?.elt
    if (!isSkillsFilterRequestElement(element)) {
      return false
    }

    if (element.matches('[data-skills-retry]')) {
      element.setAttribute('hx-get', element.dataset.retryUrl || currentSkillsStateURL())
    }

    snapshotSkillsControlFocus()
    if (element.matches('[data-skills-retry], [data-skills-clear]')) {
      skillsControlFocusSnapshot = {
        id: 'skills-search',
        search: true,
        selectionStart: 0,
        selectionEnd: 0,
        selectionDirection: 'none',
      }
    }
    activeSkillsDetailRequest = null
    const request = {
      id: ++skillsRequestSequence,
      xhr: event.detail.xhr,
      element,
      url: requestedSkillsURL(event),
      focusAfterSuccess: element.matches('[data-skills-retry], [data-skills-clear]'),
    }
    if (request.xhr) {
      request.xhr.__skillsRequestID = request.id
      bindSkillsResponseGate(element, request.xhr)
    }
    activeSkillsRequest = request
    hideSkillsFilterError()
    setSkillsBusy(true)
    setSkillsRequestState('busy')
    return true
  }

  function suppressStaleSkillsResponse(event) {
    const xhr = event.detail?.xhr
    const detailRequest = xhr ? skillsDetailRequestContext.get(xhr) : null
    if (detailRequest) {
      if (!activeSkillsDetailRequest || detailRequest.id !== activeSkillsDetailRequest.id || !detailRequest.trigger.isConnected) {
        event.detail.shouldSwap = false
        event.preventDefault()
      }
      return
    }
    if (!xhr?.__skillsRequestID) {
      return
    }

    if (!activeSkillsRequest || xhr.__skillsRequestID !== activeSkillsRequest.id) {
      event.detail.shouldSwap = false
      event.preventDefault()
    }
  }

  function bindSkillsResponseGate(element, xhr) {
    if (!(element instanceof Element) || !xhr) {
      return
    }

    const gate = event => {
      if (event.detail?.xhr !== xhr) {
        return
      }
      suppressStaleSkillsResponse(event)
      element.removeEventListener('htmx:beforeOnLoad', gate)
    }
    const cleanup = () => element.removeEventListener('htmx:beforeOnLoad', gate)
    element.addEventListener('htmx:beforeOnLoad', gate)
    xhr.addEventListener('loadend', cleanup, { once: true })
  }

  function settleSkillsFilterRequest(event, failed) {
    const request = skillsRequestRecord(event)
    if (!request) {
      return false
    }

    if (failed) {
      const retry = document.querySelector('[data-skills-retry]')
      if (retry) {
        retry.dataset.retryUrl = request.url
        retry.setAttribute('hx-get', request.url)
        window.htmx?.process(retry)
      }
      showSkillsFilterError()
      setSkillsRequestState('error')
    } else {
      hideSkillsFilterError()
      setSkillsRequestState('idle')
      if (request.focusAfterSuccess) {
        window.requestAnimationFrame(() => {
          if (skillsRequestSequence !== request.id || activeSkillsRequest) {
            return
          }
          const active = document.activeElement
          if (active && active !== document.body && active.isConnected && !active.closest('[hidden]')) {
            return
          }
          document.querySelector('[data-skills-search-input]')?.focus({ preventScroll: true })
        })
      }
    }
    activeSkillsRequest = null
    setSkillsBusy(false)
    return true
  }

  function setupSkillsInteractions() {
    setupSkillsFilterKeyboard()
  }

  function invalidateSkillsIntentOnInput(event) {
    const input = event.target.closest?.('[data-skills-search-input]')
    if (!input || !activeSkillsRequest) {
      return
    }

    const form = input.closest('form')
    const intended = new URLSearchParams(new FormData(form))
    const current = new URL(activeSkillsRequest.url, window.location.href).searchParams
    if (intended.toString() !== current.toString()) {
      const staleXHR = activeSkillsRequest.xhr
      activeSkillsRequest = null
      setSkillsBusy(false)
      setSkillsRequestState('idle')
      if (typeof staleXHR?.abort === 'function') {
        staleXHR.abort()
      }
    }
  }

  function setupSkillsFilterKeyboard() {
    document.querySelectorAll('.filter-tabs').forEach(group => {
      if (group.dataset.kbBound === 'true') {
        return
      }

      group.dataset.kbBound = 'true'
      const buttons = Array.from(group.querySelectorAll('.filter-tab'))

      buttons.forEach(btn => {
        btn.addEventListener('keydown', event => {
          if (event.key === 'Enter' || event.key === ' ') {
            skillsKeyboardActivations.add(btn)
          }
          moveFocusByArrowKey(event, buttons, buttons.indexOf(btn))
        })
      })
    })
  }

  function setupRemoteImageFallbacks(root = document) {
    const remoteImages = []

    if (root instanceof Element && root.matches('[data-remote-image]')) {
      remoteImages.push(root)
    }
    if (typeof root.querySelectorAll === 'function') {
      remoteImages.push(...root.querySelectorAll('[data-remote-image]'))
    }

    remoteImages.forEach(image => {
      const markLoaded = () => {
        image.classList.add('is-loaded')
        image.classList.remove('is-failed')
      }
      const markFailed = () => {
        image.classList.add('is-failed')
        image.classList.remove('is-loaded')
      }

      if (image.dataset.remoteImageBound !== 'true') {
        image.dataset.remoteImageBound = 'true'
        image.addEventListener('load', markLoaded)
        image.addEventListener('error', markFailed)
      }

      if (image.complete) {
        if (image.naturalWidth > 0) {
          markLoaded()
        } else {
          markFailed()
        }
      }
    })
  }

  const soccerLoginModal = document.getElementById('soccer-login-modal')
  const soccerModalBackgroundState = new Map()
  const soccerRequestKeyboardActivations = new WeakSet()
  const soccerRequestContexts = new WeakMap()
  const activeSoccerTargets = new Map()
  const latestSoccerRequestIDs = new Map()
  const portalDetailRequestContexts = new WeakMap()
  const activePortalDetailTargets = new Map()
  const latestPortalDetailRequestIDs = new Map()
  const portalPointerActivations = new WeakSet()
  const SOCCER_REQUEST_TARGET_IDS = new Set([
    'soccer-lps-connection',
    'soccer-google-connection',
    'soccer-login-feedback',
    'soccer-team-stage-content',
    'games-container',
  ])
	function isSoccerRequestTarget(target) {
		return target instanceof Element && (SOCCER_REQUEST_TARGET_IDS.has(target.id) || target.matches('[data-soccer-feedback]'))
	}

  let soccerRequestSequence = 0
  let portalDetailRequestSequence = 0
  let soccerProcessingContext = null
  let soccerLoginTrigger = null

  function setExpandedState(elements, isExpanded) {
    elements.forEach(element => {
      element.setAttribute('aria-expanded', isExpanded ? 'true' : 'false')
    })
  }

  function focusSwappedRegion(region, canFocus = () => true) {
    region.setAttribute('tabindex', '-1')
    window.requestAnimationFrame(() => {
      if (!region.isConnected || !canFocus()) {
        return
      }
      region.focus({ preventScroll: true })
      region.scrollIntoView({
        behavior: prefersReducedMotionQuery.matches ? 'auto' : 'smooth',
        block: 'start',
      })
    })
  }

  function getPortalDetailControl(element) {
    if (!(element instanceof Element)) {
      return null
    }
    return element.closest('[data-portal-detail-control]')
  }

  function setPortalOpenState(element, attribute, isOpen) {
    if (!element) {
      return
    }
    if (isOpen) {
      element.setAttribute(attribute, 'true')
    } else {
      element.removeAttribute(attribute)
    }
  }

  function syncPortalDetailRowOpen(detailRow, primaryRow) {
    const shouldRemainOpen = [...detailRow.querySelectorAll('[data-portal-detail-panel]')].some(panel => {
      const target = panel.querySelector('.portal-instance-detail-content')
      return (
        panel.getAttribute('data-portal-panel-open') === 'true' ||
        target?.childNodes.length > 0 ||
        target?.getAttribute('aria-busy') === 'true'
      )
    })
    setPortalOpenState(detailRow, 'data-portal-detail-open', shouldRemainOpen)
    setPortalOpenState(primaryRow, 'data-portal-detail-open', shouldRemainOpen)
  }

  function clearPortalFocusMarker(context) {
    const control = context?.control
    if (
      !control?.isConnected ||
      control.dataset.portalFocusRequestId !== String(context.id)
    ) {
      return
    }
    delete control.dataset.portalFocusRequestId
    control.removeAttribute('data-focus-after-swap')
  }

  function restorePortalControlFocusIfLost(context) {
    window.requestAnimationFrame(() => {
      const activeElement = document.activeElement
      const focusIsLost =
        !activeElement || activeElement === document.body || !activeElement.isConnected
      if (
        !focusIsLost ||
        !context.control.isConnected ||
        latestPortalDetailRequestIDs.get(context.targetID) !== context.id
      ) {
        return
      }
      context.control.focus({ preventScroll: true })
    })
  }

  function beginPortalDetailRequest(evt) {
    const control = getPortalDetailControl(evt.detail?.elt)
    const xhr = evt.detail?.xhr
    const targetID = control?.getAttribute('aria-controls')
    const target = targetID ? document.getElementById(targetID) : null
    const panel = target?.closest('[data-portal-detail-panel]')
    const detailRow = target?.closest('[data-portal-instance-detail]')
    const primaryRow = detailRow?.previousElementSibling?.matches('.portal-instance-row')
      ? detailRow.previousElementSibling
      : null
    if (!control || !xhr || !target || !panel || !detailRow || !primaryRow) {
      return
    }

    const predecessor = activePortalDetailTargets.get(targetID)
    const liveBaseline = {
      hadContent: target.childNodes.length > 0,
      wasExpanded: control.getAttribute('aria-expanded') === 'true',
      wasPanelOpen: panel.getAttribute('data-portal-panel-open') === 'true',
    }
    const baselineSource =
      predecessor && !predecessor.swapped ? predecessor.stableBaseline : liveBaseline
    const stableBaseline = { ...baselineSource }
    const context = {
      id: ++portalDetailRequestSequence,
      xhr,
      control,
      targetID,
      panel,
      detailRow,
      primaryRow,
      loading: panel.querySelector('[data-portal-detail-loading]'),
      stableBaseline,
      retainControlFocus: portalPointerActivations.has(control),
      swapped: false,
    }
    portalPointerActivations.delete(control)
    portalDetailRequestContexts.set(xhr, context)
    activePortalDetailTargets.set(targetID, context)
    latestPortalDetailRequestIDs.set(targetID, context.id)
    target.dataset.portalDetailRequestId = String(context.id)
    target.setAttribute('aria-busy', 'true')
    setPortalOpenState(panel, 'data-portal-panel-open', true)
    setPortalOpenState(detailRow, 'data-portal-detail-open', true)
    setPortalOpenState(primaryRow, 'data-portal-detail-open', true)
    if (context.loading) {
      context.loading.hidden = false
    }
    if (control.getAttribute('data-focus-after-swap') === 'true') {
      control.dataset.portalFocusRequestId = String(context.id)
    }
  }

  function completePortalDetailSwap(evt) {
    const xhr = evt.detail?.xhr
    const context = xhr ? portalDetailRequestContexts.get(xhr) : null
    const requestControl = getPortalDetailControl(evt.detail?.requestConfig?.elt)
    const target = evt.detail?.target
    if (
      !context ||
      !(target instanceof Element) ||
      target.id !== context.targetID ||
      requestControl !== context.control ||
      activePortalDetailTargets.get(context.targetID)?.id !== context.id
    ) {
      return
    }

    context.swapped = true
    target.setAttribute('aria-busy', 'false')
    context.control.setAttribute('aria-expanded', 'true')
    setPortalOpenState(context.panel, 'data-portal-panel-open', true)
    setPortalOpenState(context.detailRow, 'data-portal-detail-open', true)
    setPortalOpenState(context.primaryRow, 'data-portal-detail-open', true)
    if (context.loading) {
      context.loading.hidden = true
    }

    const shouldFocus =
      context.control.getAttribute('data-focus-after-swap') === 'true' &&
      context.control.dataset.portalFocusRequestId === String(context.id)
    clearPortalFocusMarker(context)
    if (shouldFocus) {
      focusSwappedRegion(
        target,
        () =>
          target.isConnected &&
          context.control.isConnected &&
          context.control.getAttribute('aria-expanded') === 'true' &&
          latestPortalDetailRequestIDs.get(context.targetID) === context.id
      )
    } else if (context.retainControlFocus) {
      restorePortalControlFocusIfLost(context)
    }
  }

  function suppressStalePortalDetailResponse(evt) {
    const xhr = evt.detail?.xhr
    const context = xhr ? portalDetailRequestContexts.get(xhr) : null
    if (!context) {
      return
    }
    if (activePortalDetailTargets.get(context.targetID)?.id !== context.id) {
      evt.detail.shouldSwap = false
      evt.preventDefault()
    }
  }

  function settlePortalDetailRequest(evt) {
    const xhr = evt.detail?.xhr
    const eventControl = getPortalDetailControl(evt.detail?.elt)
    const eventTargetID = eventControl?.getAttribute('aria-controls')
    const context = xhr
      ? portalDetailRequestContexts.get(xhr)
      : eventTargetID && activePortalDetailTargets.get(eventTargetID)
    if (!context) {
      return
    }

    if (activePortalDetailTargets.get(context.targetID)?.id === context.id) {
      activePortalDetailTargets.delete(context.targetID)
      const target = document.getElementById(context.targetID)
      if (target?.dataset.portalDetailRequestId === String(context.id)) {
        delete target.dataset.portalDetailRequestId
      }
      target?.setAttribute('aria-busy', 'false')
      if (context.loading) {
        context.loading.hidden = true
      }
      if (!context.swapped) {
        const restoreExpanded =
          context.stableBaseline.wasExpanded || context.stableBaseline.hadContent
        const restorePanelOpen =
          context.stableBaseline.wasPanelOpen || context.stableBaseline.hadContent
        context.control.setAttribute('aria-expanded', restoreExpanded ? 'true' : 'false')
        setPortalOpenState(context.panel, 'data-portal-panel-open', restorePanelOpen)
        syncPortalDetailRowOpen(context.detailRow, context.primaryRow)
        if (context.retainControlFocus) {
          restorePortalControlFocusIfLost(context)
        }
      }
    }
    clearPortalFocusMarker(context)
    if (xhr) {
      portalDetailRequestContexts.delete(xhr)
    }
  }

  document.addEventListener('keydown', event => {
    if (event.key !== 'Enter' && event.key !== ' ') {
      return
    }
    const control = getPortalDetailControl(event.target)
    if (control) {
      portalPointerActivations.delete(control)
      control.setAttribute('data-focus-after-swap', 'true')
    }
  })

  document.addEventListener('pointerdown', event => {
    const control = getPortalDetailControl(event.target)
    if (!control) {
      return
    }
    portalPointerActivations.add(control)
    control.removeAttribute('data-focus-after-swap')
    delete control.dataset.portalFocusRequestId
  })

  function closeOtherSkillDetails(activeSlot, activeTrigger) {
    document.querySelectorAll('[data-skill-detail-trigger][aria-expanded="true"]').forEach(trigger => {
      if (trigger !== activeTrigger) {
        trigger.setAttribute('aria-expanded', 'false')
      }
    })
    document.querySelectorAll('.skill-detail-slot').forEach(slot => {
      if (slot === activeSlot) {
        return
      }
      slot.replaceChildren()
      if (slot.id) {
        setExpandedState(document.querySelectorAll(`[aria-controls="${slot.id}"]`), false)
      }
    })
  }

  function scrollSkillsElementIntoView(element, requiredElements = [element], instant = false) {
    if (!element?.isConnected) {
      return
    }

    const header = document.querySelector('.site-chrome-header')
    const headerBottom = header?.getBoundingClientRect().bottom || 0
    const viewportBottom = window.innerHeight || document.documentElement.clientHeight
    const fullyVisible = requiredElements.filter(candidate => candidate?.isConnected).every(candidate => {
      const rect = candidate.getBoundingClientRect()
      return rect.top >= headerBottom && rect.bottom <= viewportBottom
    })
    if (fullyVisible) {
      return
    }

    const anchor = requiredElements.find(candidate => candidate?.isConnected) || element
    const rect = anchor.getBoundingClientRect()
    const clearance = Number.parseFloat(window.getComputedStyle(document.documentElement).fontSize) || 16
    window.scrollTo({
      top: Math.max(0, window.scrollY + rect.top - headerBottom - clearance),
      behavior: instant || prefersReducedMotionQuery.matches ? 'auto' : 'smooth',
    })
  }

  document.addEventListener('keydown', event => {
    const trigger = event.target.closest?.('[data-skill-detail-trigger]')
    if (trigger && (event.key === 'Enter' || event.key === ' ')) {
      skillsKeyboardActivations.add(trigger)
    }
  })

  document.addEventListener('pointerdown', event => {
    const trigger = event.target.closest?.('[data-skill-detail-trigger]')
    if (trigger) {
      skillsKeyboardActivations.delete(trigger)
    }
  })

  document.addEventListener('input', invalidateSkillsIntentOnInput)

  function getModalFocusableElements() {
    if (!soccerLoginModal) {
      return []
    }

    return Array.from(soccerLoginModal.querySelectorAll(MODAL_FOCUSABLE_SELECTOR)).filter(
      element => !element.hidden
    )
  }

  function setSoccerModalVisibility(isVisible) {
    if (!soccerLoginModal) {
      return
    }

    soccerLoginModal.hidden = !isVisible
    soccerLoginModal.setAttribute('aria-hidden', isVisible ? 'false' : 'true')
    document.body.classList.toggle('soccer-modal-open', isVisible)
    setOverlayBackgroundInert([soccerLoginModal], soccerModalBackgroundState, isVisible)
  }

  function openSoccerLoginModal(trigger) {
    if (!soccerLoginModal) {
      return
    }

    soccerLoginTrigger = trigger || document.activeElement
    setSoccerModalVisibility(true)

    const dialog = soccerLoginModal.querySelector('.soccer-login-dialog')
    const firstField = soccerLoginModal.querySelector('#soccer-import-jwt')
    window.requestAnimationFrame(() => {
      ;(firstField || dialog)?.focus()
    })
  }

  function closeSoccerLoginModal() {
    if (!soccerLoginModal) {
      return
    }

    setSoccerModalVisibility(false)

    const previousTrigger = soccerLoginTrigger
    soccerLoginTrigger = null

    if (previousTrigger?.isConnected && typeof previousTrigger.focus === 'function') {
      previousTrigger.focus()
      return
    }

    const currentSoccerControl = document.querySelector(
      '#soccer-player-select-form input:not([type="hidden"]):not([disabled]), #soccer-lps-connection button:not([disabled]), #soccer-lps-connection a[href]'
    )
    if (currentSoccerControl && typeof currentSoccerControl.focus === 'function') {
      currentSoccerControl.focus()
      return
    }

    document.getElementById('maincontent')?.focus()
  }

  function resetSoccerResults() {
    activeSoccerTargets.clear()
    latestSoccerRequestIDs.clear()
    SOCCER_REQUEST_TARGET_IDS.forEach(targetID => {
      const target = document.getElementById(targetID)
      target?.removeAttribute('aria-busy')
      if (target) {
        delete target.dataset.soccerRequestId
      }
    })
    document.getElementById('soccer-results-announcer')?.replaceChildren()
    setupSoccerSelectAll()
  }

  function setSoccerLoadingState(control, isLoading) {
    if (!control) {
      return
    }

    control.classList.toggle('is-loading', isLoading)
		const buttonText = control.querySelector?.('.btn-text')
		if (buttonText && control.dataset.loadingText) {
			if (isLoading) {
				if (!Object.prototype.hasOwnProperty.call(control.dataset, 'loadingOriginalText')) {
					control.dataset.loadingOriginalText = buttonText.textContent
				}
				const gameGroup = control.dataset.gameGroup
				const selectedCount = gameGroup
					? document.querySelectorAll(`[data-game-checkbox][data-game-group="${gameGroup}"]:checked`).length
					: 0
				buttonText.textContent = control.dataset.loadingText.replace('{count}', String(selectedCount))
			} else if (Object.prototype.hasOwnProperty.call(control.dataset, 'loadingOriginalText')) {
				buttonText.textContent = control.dataset.loadingOriginalText
				delete control.dataset.loadingOriginalText
			}
		}

    if (control.tagName === 'A') {
      if (isLoading) {
        control.dataset.loading = 'true'
        control.setAttribute('aria-disabled', 'true')
        control.setAttribute('aria-busy', 'true')
      } else {
        delete control.dataset.loading
        control.removeAttribute('aria-disabled')
        control.removeAttribute('aria-busy')
      }
      return
    }

    if (!Object.prototype.hasOwnProperty.call(control.dataset, 'loadingWasDisabled')) {
      control.dataset.loadingWasDisabled = control.disabled ? 'true' : 'false'
    }

    if (isLoading) {
      control.disabled = true
      control.setAttribute('aria-busy', 'true')
      return
    }

    if (control.dataset.loadingWasDisabled !== 'true') {
      control.disabled = false
    }

    control.removeAttribute('aria-busy')
    delete control.dataset.loadingWasDisabled

    const gameGroup = control.dataset.gameGroup
    if (
      gameGroup &&
      !document.querySelector(`[data-game-checkbox][data-game-group="${gameGroup}"]:checked`)
    ) {
      control.disabled = true
    }
  }

  function getSoccerLoadingControl(trigger) {
    if (!trigger || !(trigger instanceof Element)) {
      return null
    }

    if (trigger.matches('[data-loading-button]')) {
      return trigger
    }

    if (trigger.matches('form')) {
      return trigger.querySelector('[data-loading-button]')
    }

    return trigger.closest('form')?.querySelector('[data-loading-button]') || null
  }

  function getSoccerRequestTarget(evt) {
    const soccerTarget = evt.detail?.target || evt.detail?.requestConfig?.target
		if (!isSoccerRequestTarget(soccerTarget)) {
      return null
    }
    return soccerTarget
  }

  function getSoccerRequestElement(element) {
    if (!(element instanceof Element)) {
      return null
    }
    return element.closest('.soccer-page [hx-get], .soccer-page [hx-post]')
  }

  function markSoccerKeyboardActivation(element) {
    if (!(element instanceof Element)) {
      return
    }
    soccerRequestKeyboardActivations.add(element)
    if (element.form) {
      soccerRequestKeyboardActivations.add(element.form)
    }
  }

  function clearSoccerKeyboardActivation(element) {
    if (!(element instanceof Element)) {
      return
    }
    soccerRequestKeyboardActivations.delete(element)
    if (element.form) {
      soccerRequestKeyboardActivations.delete(element.form)
    }
  }

  function beginSoccerRequest(evt) {
    const soccerTarget = getSoccerRequestTarget(evt)
    const xhr = evt.detail?.xhr
    if (!soccerTarget || !xhr) {
      return
    }

    const requestElement = evt.detail.elt
    const loadingControl = getSoccerLoadingControl(requestElement)
    const context = {
      id: ++soccerRequestSequence,
      xhr,
      targetID: soccerTarget.id,
      loadingControl,
      keyboard:
        soccerRequestKeyboardActivations.has(requestElement) ||
        soccerRequestKeyboardActivations.has(loadingControl),
    }
    soccerRequestContexts.set(xhr, context)
    activeSoccerTargets.set(context.targetID, context)
    latestSoccerRequestIDs.set(context.targetID, context.id)
    xhr.__soccerRequestID = context.id
    bindSoccerResponseGate(requestElement, xhr)
    soccerTarget.dataset.soccerRequestId = String(context.id)
    soccerTarget.setAttribute('aria-busy', 'true')
    if (loadingControl) {
      loadingControl.dataset.soccerLoadingRequestId = String(context.id)
      setSoccerLoadingState(loadingControl, true)
			const message = loadingControl.dataset.loadingText
			if (message && soccerTarget.matches('[data-soccer-feedback]')) {
				const gameGroup = loadingControl.dataset.gameGroup
				const selectedCount = gameGroup
					? document.querySelectorAll(`[data-game-checkbox][data-game-group="${gameGroup}"]:checked`).length
					: 0
				soccerTarget.textContent = message.replace('{count}', String(selectedCount))
			}
    }
    clearSoccerKeyboardActivation(requestElement)
    clearSoccerKeyboardActivation(loadingControl)
  }

  function settleSoccerRequest(evt) {
    const xhr = evt.detail?.xhr
    const eventTarget = getSoccerRequestTarget(evt)
    const context = xhr
      ? soccerRequestContexts.get(xhr)
      : eventTarget && activeSoccerTargets.get(eventTarget.id)
    if (!context) {
      return
    }

    const active = activeSoccerTargets.get(context.targetID)
    if (active?.id === context.id) {
      activeSoccerTargets.delete(context.targetID)
      const soccerTarget = document.getElementById(context.targetID)
      if (soccerTarget?.dataset.soccerRequestId === String(context.id)) {
        delete soccerTarget.dataset.soccerRequestId
        soccerTarget.removeAttribute('aria-busy')
      } else {
        soccerTarget?.removeAttribute('aria-busy')
      }
    }
    if (
      context.loadingControl?.isConnected &&
      context.loadingControl.dataset.soccerLoadingRequestId === String(context.id)
    ) {
      delete context.loadingControl.dataset.soccerLoadingRequestId
      setSoccerLoadingState(context.loadingControl, false)
    }
    if (xhr) {
      soccerRequestContexts.delete(xhr)
    }
  }

  function suppressStaleSoccerResponse(evt) {
    const xhr = evt.detail?.xhr
    const context = xhr ? soccerRequestContexts.get(xhr) : null
    if (!context) {
      return
    }

    const active = activeSoccerTargets.get(context.targetID)
    if (!active || active.id !== context.id) {
      evt.detail.shouldSwap = false
      evt.preventDefault()
    }
  }

  function bindSoccerResponseGate(element, xhr) {
    if (!(element instanceof Element) || !xhr) {
      return
    }

    const gate = evt => {
      if (evt.detail?.xhr !== xhr) {
        return
      }
      soccerProcessingContext = soccerRequestContexts.get(xhr) || null
      suppressStaleSoccerResponse(evt)
      element.removeEventListener('htmx:beforeOnLoad', gate)
    }
    const cleanup = () => {
      element.removeEventListener('htmx:beforeOnLoad', gate)
      if (soccerProcessingContext?.xhr === xhr) {
        soccerProcessingContext = null
      }
    }
    element.addEventListener('htmx:beforeOnLoad', gate)
    xhr.addEventListener('loadend', cleanup, { once: true })
  }

  function announceSoccerTargetUpdate(target, context) {
		if (!isSoccerRequestTarget(target)) {
      return
    }

    const announcer = document.getElementById('soccer-results-announcer')
    if (announcer && ['soccer-team-stage-content', 'games-container'].includes(target.id)) {
      const message = target.id === 'games-container' ? 'Schedule results updated.' : 'Team choices updated.'
      announcer.replaceChildren()
      window.requestAnimationFrame(() => {
        if (
          announcer.isConnected &&
          latestSoccerRequestIDs.get(context.targetID) === context.id &&
          document.getElementById(context.targetID) === target
        ) {
          announcer.textContent = message
        }
      })
    }
    if (context?.keyboard && ['soccer-team-stage-content', 'games-container'].includes(target.id)) {
      const currentTarget = document.getElementById(context.targetID)
      if (!currentTarget) {
        return
      }
      focusSwappedRegion(
        currentTarget,
        () =>
          latestSoccerRequestIDs.get(context.targetID) === context.id &&
          document.getElementById(context.targetID) === currentTarget
      )
    }
  }

  function invalidateDisplacedSoccerTarget(targetID) {
    const displaced = activeSoccerTargets.get(targetID)
    if (!displaced) {
      return
    }

    activeSoccerTargets.delete(targetID)
    latestSoccerRequestIDs.delete(targetID)
    const currentTarget = document.getElementById(targetID)
    if (currentTarget) {
      delete currentTarget.dataset.soccerRequestId
      currentTarget.removeAttribute('aria-busy')
    }
    if (
      displaced.loadingControl?.isConnected &&
      displaced.loadingControl.dataset.soccerLoadingRequestId === String(displaced.id)
    ) {
      delete displaced.loadingControl.dataset.soccerLoadingRequestId
      setSoccerLoadingState(displaced.loadingControl, false)
    }
  }

  function clearSoccerProcessingContext(evt) {
    if (soccerProcessingContext?.xhr === evt.detail?.xhr) {
      soccerProcessingContext = null
    }
  }

  document.addEventListener('keydown', event => {
    if (event.key !== 'Enter' && event.key !== ' ') {
      return
    }
    markSoccerKeyboardActivation(getSoccerRequestElement(event.target))
  })

  document.addEventListener('pointerdown', event => {
    clearSoccerKeyboardActivation(getSoccerRequestElement(event.target))
  })

  function resetSoccerLoadingLinks() {
    document.querySelectorAll('[data-loading-link][data-loading="true"]').forEach(link => {
      setSoccerLoadingState(link, false)
    })
  }

  function setupSoccerLoginModal() {
    if (!soccerLoginModal || soccerLoginModal.dataset.bound === 'true') {
      return
    }

    soccerLoginModal.dataset.bound = 'true'

    document.addEventListener('click', event => {
      const openButton = event.target.closest('[data-open-login-modal]')
      if (openButton) {
        event.preventDefault()
        openSoccerLoginModal(openButton)
        return
      }

      if (event.target.closest('[data-close-login-modal]')) {
        event.preventDefault()
        closeSoccerLoginModal()
      }
    })

    document.addEventListener('keydown', event => {
      if (!soccerLoginModal || soccerLoginModal.hidden) {
        return
      }

      if (event.key === 'Escape') {
        event.preventDefault()
        closeSoccerLoginModal()
        return
      }

      if (event.key !== 'Tab') {
        return
      }

      const focusableElements = getModalFocusableElements()
      if (focusableElements.length === 0) {
        return
      }

      const firstElement = focusableElements[0]
      const lastElement = focusableElements[focusableElements.length - 1]

      if (event.shiftKey && document.activeElement === firstElement) {
        event.preventDefault()
        lastElement.focus()
      } else if (!event.shiftKey && document.activeElement === lastElement) {
        event.preventDefault()
        firstElement.focus()
      }
    })
  }

  function initializeServerOpenSoccerModal() {
    if (!soccerLoginModal || soccerLoginModal.hidden || soccerLoginModal.getAttribute('aria-hidden') === 'true') {
      return
    }

    setSoccerModalVisibility(true)
    const dialog = soccerLoginModal.querySelector('.soccer-login-dialog')
    const firstField = soccerLoginModal.querySelector('#soccer-import-jwt')
    window.requestAnimationFrame(() => {
      ;(firstField || dialog)?.focus()
    })
  }

  // HTMX event handlers
  document.body.addEventListener('htmx:beforeOnLoad', suppressStaleSkillsResponse)
  document.body.addEventListener('htmx:beforeOnLoad', suppressStaleSoccerResponse)
  document.body.addEventListener('htmx:beforeOnLoad', suppressStalePortalDetailResponse)

  document.body.addEventListener('htmx:afterSwap', function (evt) {
    // Fade in new content
    if (evt.detail.target && !prefersReducedMotionQuery.matches) {
      evt.detail.target.classList.add('fade-in')
    }

    setupRemoteImageFallbacks(evt.detail.target)

    // Soccer page specific handlers - check for soccer form using data attribute
    if (evt.target.querySelector('[data-soccer-form]') || evt.target.id === 'games-container') {
      setupSoccerSelectAll()
    }

    const soccerContext = evt.detail.xhr ? soccerRequestContexts.get(evt.detail.xhr) : null
    const swappedSoccerTarget =
			isSoccerRequestTarget(evt.target) ? evt.target : null
    if (
      soccerContext &&
      swappedSoccerTarget?.id === soccerContext.targetID &&
      activeSoccerTargets.get(soccerContext.targetID)?.id === soccerContext.id
    ) {
      swappedSoccerTarget.dataset.soccerRequestId = String(soccerContext.id)
      swappedSoccerTarget.setAttribute('aria-busy', 'true')
      announceSoccerTargetUpdate(swappedSoccerTarget, soccerContext)
    }

    completePortalDetailSwap(evt)

    if (
      evt.target instanceof Element &&
      evt.target.id === 'soccer-login-feedback' &&
      document.getElementById('soccer-login-feedback') === evt.target &&
      evt.target.querySelector('[data-login-success]')
    ) {
      const loginForm = document.getElementById('soccer-login-form')
      if (loginForm) {
        loginForm.reset()
      }
      closeSoccerLoginModal()
    }

    setupSkillsInteractions()

    if (evt.detail.target.id === 'skills-filter-controls') {
      restoreSkillsControlFocus()
    }

    // HTMX reports the swap target as `elt` for an innerHTML swap. The original
    // control remains available on requestConfig, which keeps its expanded state
    // and keyboard focus accurate after the detail fragment arrives.
    const requestElement = evt.detail.requestConfig?.elt || evt.detail.elt
    const detailContext = evt.detail.xhr ? skillsDetailRequestContext.get(evt.detail.xhr) : null
    if (
      requestElement?.matches('[data-skill-detail-trigger]') &&
      detailContext?.trigger === requestElement &&
      requestElement.isConnected &&
      evt.detail.target.classList.contains('skill-detail-slot') &&
      evt.detail.target.id === detailContext.slotID
    ) {
      closeOtherSkillDetails(evt.detail.target, requestElement)
      requestElement.setAttribute('aria-expanded', 'true')
      const heading = evt.detail.target.querySelector('[data-skill-detail-heading]')
      window.requestAnimationFrame(() => {
        if (
          heading?.isConnected &&
          activeSkillsDetailRequest?.id === detailContext.id &&
          detailContext.trigger.isConnected &&
          detailContext.trigger.getAttribute('aria-expanded') === 'true' &&
          evt.detail.target.contains(heading)
        ) {
          if (detailContext.keyboard) {
            heading.focus({ preventScroll: true })
          }
          const detailCard = heading.closest('.skill-detail-card') || heading
          const close = detailCard.querySelector('[data-close-detail]')
          const ensureDetailVisible = instant => {
            if (
              activeSkillsDetailRequest?.id === detailContext.id &&
              detailContext.trigger.isConnected &&
              detailContext.trigger.getAttribute('aria-expanded') === 'true' &&
              detailCard.isConnected &&
              evt.detail.target.id === detailContext.slotID &&
              evt.detail.target.contains(detailCard) &&
              evt.detail.target.contains(heading)
            ) {
              scrollSkillsElementIntoView(detailCard, [close, heading], instant)
            }
          }
          ensureDetailVisible(false)
          window.setTimeout(() => ensureDetailVisible(true), 450)
          window.setTimeout(() => ensureDetailVisible(true), 950)
        }
      })
    }

    // Skills page: re-observe new skill categories after filter swap
    if (evt.detail.target.id === 'skills-filterable') {
      const newCategories = evt.detail.target.querySelectorAll('.skill-category')
      if (observer) {
        newCategories.forEach(function (el) {
          observer.observe(el)
        })
      }
    }
  })

  // Skills page: close all detail panels before opening a new one
  document.body.addEventListener('htmx:beforeRequest', function (evt) {
    beginPortalDetailRequest(evt)
    beginSoccerRequest(evt)

    beginSkillsFilterRequest(evt)

    if (evt.detail.elt?.matches('[data-skill-detail-trigger]')) {
      const keyboard = skillsKeyboardActivations.has(evt.detail.elt)
      skillsKeyboardActivations.delete(evt.detail.elt)
      if (evt.detail.xhr) {
        const context = {
          id: ++skillsDetailRequestSequence,
          trigger: evt.detail.elt,
          slotID: evt.detail.elt.getAttribute('aria-controls'),
          keyboard,
        }
        skillsDetailRequestContext.set(evt.detail.xhr, context)
        activeSkillsDetailRequest = context
        bindSkillsResponseGate(evt.detail.elt, evt.detail.xhr)
      }
    }
  })

  document.body.addEventListener('htmx:afterRequest', function (evt) {
    settlePortalDetailRequest(evt)
    settleSoccerRequest(evt)
    clearSoccerProcessingContext(evt)
    if (isActiveSkillsRequest(evt) && evt.detail.successful === true) {
      settleSkillsFilterRequest(evt, false)
    } else if (isActiveSkillsRequest(evt) && evt.detail.successful === false) {
      settleSkillsFilterRequest(evt, true)
    }
  })

  ;['htmx:responseError', 'htmx:sendError', 'htmx:timeout'].forEach(eventName => {
    document.body.addEventListener(eventName, function (evt) {
      settlePortalDetailRequest(evt)
      settleSoccerRequest(evt)
      clearSoccerProcessingContext(evt)
      settleSkillsFilterRequest(evt, true)
    })
  })

  document.body.addEventListener('htmx:sendAbort', function (evt) {
    settlePortalDetailRequest(evt)
    settleSoccerRequest(evt)
    clearSoccerProcessingContext(evt)
    if (isActiveSkillsRequest(evt)) {
      activeSkillsRequest = null
      setSkillsBusy(false)
      setSkillsRequestState('idle')
    }
  })

  document.body.addEventListener('htmx:oobBeforeSwap', function (evt) {
    const target = evt.detail?.target instanceof Element ? evt.detail.target : evt.target
    const processing = soccerProcessingContext
		if (!isSoccerRequestTarget(target) || !processing) {
      return
    }

    const latestTargetRequestID = latestSoccerRequestIDs.get(target.id) || 0
    if (latestTargetRequestID > 0 && processing.id < latestTargetRequestID) {
      evt.detail.shouldSwap = false
      evt.preventDefault()
      return
    }

    const displaced = activeSoccerTargets.get(target.id)
    if (displaced && displaced.id !== processing.id) {
      invalidateDisplacedSoccerTarget(target.id)
    }
    latestSoccerRequestIDs.set(target.id, processing.id)
  })

  document.body.addEventListener('htmx:oobAfterSwap', function (evt) {
    const swappedTarget = evt.detail?.target instanceof Element ? evt.detail.target : evt.target
		if (!isSoccerRequestTarget(swappedTarget)) {
      return
    }

    const soccerTarget = document.getElementById(swappedTarget.id)
    if (!soccerTarget) {
      return
    }

    delete soccerTarget.dataset.soccerRequestId
    soccerTarget.removeAttribute('aria-busy')
  })

  document.body.addEventListener('htmx:afterProcessNode', function (evt) {
    if (evt.detail.elt?.id === 'skills-filter-controls') {
      restoreSkillsControlFocus()
    }
  })

  document.body.addEventListener('htmx:historyRestore', function () {
    const historyFocus = document.querySelector('[data-skills-history-focus]')
    const serializedFocus = historyFocus?.getAttribute('data-skills-focus-snapshot')
    if (serializedFocus) {
      try {
        skillsControlFocusSnapshot = JSON.parse(serializedFocus)
      } catch (_error) {
        skillsControlFocusSnapshot = null
      }
      historyFocus.removeAttribute('data-skills-focus-snapshot')
    }
    activeSkillsDetailRequest = null
    resetSkillsTransientState()
    clearSkillsBindingMarkers(document)
    setupRemoteImageFallbacks(document)
    setupSkillsInteractions()
    restoreSkillsControlFocus()
  })

  document.body.addEventListener('htmx:beforeHistorySave', function () {
    snapshotSkillsControlFocus()
  })

  document.addEventListener('click', event => {
    const closeDetailButton = event.target.closest('[data-close-detail]')
    if (!closeDetailButton) {
      return
    }

    const detailSlot = closeDetailButton.closest('.skill-detail-slot')
    if (detailSlot) {
      let focusTarget = null
      let controlledButtons = []

      if (detailSlot.id) {
        controlledButtons = Array.from(document.querySelectorAll(`[aria-controls="${detailSlot.id}"]`))
        controlledButtons.forEach(btn => {
          if (btn.getAttribute('aria-expanded') === 'true') {
            focusTarget = btn
          }
        })
      }

      setExpandedState(controlledButtons, false)
      detailSlot.replaceChildren()
      if (activeSkillsDetailRequest?.slotID === detailSlot.id) {
        activeSkillsDetailRequest = null
      }

      if (focusTarget) {
        focusTarget.focus({ preventScroll: true })
        window.requestAnimationFrame(() => {
          if (document.activeElement === focusTarget) {
            scrollSkillsElementIntoView(focusTarget)
          }
        })
      }
    }
  })

  document.addEventListener('click', event => {
    const loadingLink = event.target.closest('[data-loading-link]')
    if (!loadingLink) {
      return
    }

    if (
      event.defaultPrevented ||
      event.button !== 0 ||
      event.metaKey ||
      event.ctrlKey ||
      event.shiftKey ||
      event.altKey ||
      loadingLink.target === '_blank'
    ) {
      return
    }

    if (loadingLink.dataset.loading === 'true' || loadingLink.getAttribute('aria-disabled') === 'true') {
      event.preventDefault()
      return
    }

    setSoccerLoadingState(loadingLink, true)
  })

  document.addEventListener('submit', event => {
    if (!(event.target instanceof HTMLFormElement) || event.defaultPrevented) {
      return
    }

    if (event.target.matches('[data-native-download]')) {
      return
    }

    const loadingControl = getSoccerLoadingControl(event.submitter || event.target)
    if (!loadingControl || loadingControl.getAttribute('aria-busy') === 'true') {
      return
    }

    setSoccerLoadingState(loadingControl, true)
  })

	function resetSoccerWorkflowState() {
		clearSoccerSelection()
		resetSoccerResults()
	}

	document.body.addEventListener('soccer-logout', resetSoccerWorkflowState)
	document.body.addEventListener('soccer-workflow-reset', resetSoccerWorkflowState)

  window.addEventListener('pageshow', resetSoccerLoadingLinks)

  // Initialize on page load (for non-HTMX scenarios)
  setupSoccerSelectAll()
  setupSoccerLoginModal()
  initializeServerOpenSoccerModal()
  resetSoccerLoadingLinks()
  COUNTER_SECTIONS.forEach(({ section, duration, threshold }) => {
    observeCounterSection(section, '[data-counter]', duration, threshold)
  })
  setupSkillsInteractions()
  setupRemoteImageFallbacks()

  // Add intersection observer for scroll animations
  const observerOptions = {
    root: null,
    rootMargin: '0px',
    threshold: 0.1,
  }

  if (!prefersReducedMotionQuery.matches) {
    observer = new IntersectionObserver(entries => {
      entries.forEach(entry => {
        if (entry.isIntersecting) {
          entry.target.classList.add('fade-in')
          observer?.unobserve(entry.target)
        }
      })
    }, observerOptions)

    // Observe elements that should animate on scroll
    document.querySelectorAll('.timeline-item, .project-case-study, .skill-category').forEach(el => {
      observer?.observe(el)
    })
  }

  // Header scroll behavior
  const header = document.querySelector('.site-header')

  if (header) {
    let headerScrolled = false
    const syncHeaderScrolledState = () => {
      const shouldBeScrolled = window.scrollY > HEADER_SCROLL_SHADOW_THRESHOLD
      if (shouldBeScrolled === headerScrolled) {
        return
      }

      headerScrolled = shouldBeScrolled
      header.classList.toggle('site-header-scrolled', shouldBeScrolled)
    }

    window.addEventListener('scroll', syncHeaderScrolledState, { passive: true })
    syncHeaderScrolledState()
  }
})()
