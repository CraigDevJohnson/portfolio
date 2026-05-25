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
    { section: '.about-stats', duration: 2000 },
    { section: '.experience-hero-stats', duration: 1500 },
    { section: '.experience-summary', duration: 1500 },
    { section: '.edu-stats', duration: 1500 },
    { section: '.projects-stats', duration: 1500 },
  ]

  // Mobile menu toggle
  const mobileMenuBtn = document.getElementById('mobile-menu-btn')
  const mobileNav = document.getElementById('mobile-nav')
  const mobileNavBreakpoint = window.matchMedia('(max-width: 1120px)')
  let observer = null

  if (mobileMenuBtn && mobileNav) {
    const setMobileNavState = isOpen => {
      const nextState = Boolean(isOpen) && mobileNavBreakpoint.matches

      mobileMenuBtn.setAttribute('aria-expanded', String(nextState))
      mobileNav.classList.toggle('hidden', !nextState)
      mobileNav.classList.toggle('flex', nextState)
      mobileNav.setAttribute('aria-hidden', String(!nextState))
      document.documentElement.classList.toggle('overflow-hidden', nextState)
      document.documentElement.classList.toggle('overscroll-none', nextState)
      document.body.classList.toggle('overflow-hidden', nextState)
      document.body.classList.toggle('overscroll-none', nextState)

      if (nextState) {
        mobileNav.scrollTop = 0
      }
    }

    const isMobileNavOpen = () => mobileNav.classList.contains('flex') && !mobileNav.classList.contains('hidden')

    const closeMobileNav = () => {
      setMobileNavState(false)
    }

    mobileMenuBtn.addEventListener('click', () => {
      setMobileNavState(!isMobileNavOpen())
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

    document.addEventListener('keydown', e => {
      if (e.key === 'Escape' && isMobileNavOpen()) {
        closeMobileNav()
        mobileMenuBtn.focus()
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

  // Smooth scroll for anchor links (skip links exempt for accessibility)
  document.querySelectorAll('a[href^="#"]:not(.skip-link)').forEach(anchor => {
    anchor.addEventListener('click', function (e) {
      e.preventDefault()
      const target = document.querySelector(this.getAttribute('href'))
      if (target) {
        target.scrollIntoView({
          behavior: 'smooth',
          block: 'start',
        })
      }
    })
  })

  // Soccer page functionality - using data attributes for flexibility
  function setupSoccerSelectAll() {
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

      const syncSelectAllState = () => {
        const gameCheckboxes = getGroupCheckboxes()
        if (gameCheckboxes.length === 0) {
          selectAll.checked = false
          return
        }

        selectAll.checked = gameCheckboxes.every(checkbox => checkbox.checked)
      }

      selectAll.dataset.bound = 'true'

      selectAll.addEventListener('change', function () {
        getGroupCheckboxes().forEach(checkbox => {
          checkbox.checked = selectAll.checked
        })
      })

      getGroupCheckboxes().forEach(checkbox => {
        if (checkbox.dataset.selectBound === 'true') {
          return
        }

        checkbox.dataset.selectBound = 'true'
        checkbox.addEventListener('change', syncSelectAllState)
      })

      syncSelectAllState()
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
      counter.textContent = Math.floor(currentValue + (finalValue - currentValue) * ease)

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

  function setupProjectsCategoryFilter() {
    const pills = document.querySelectorAll('.proj-category-pill')

    if (pills.length === 0) {
      return
    }

    pills.forEach(pill => {
      if (pill.dataset.bound === 'true') {
        return
      }

      pill.dataset.bound = 'true'
      pill.addEventListener('click', () => {
        pills.forEach(candidate => {
          candidate.classList.remove('active')
          candidate.setAttribute('aria-pressed', 'false')
        })

        pill.classList.add('active')
        pill.setAttribute('aria-pressed', 'true')

        const category = pill.dataset.category
        document.querySelectorAll('.project-card').forEach(card => {
          if (category === 'all') {
            card.style.display = ''
            return
          }

          const cardCategory = (card.dataset.category || '').toLowerCase()
          card.style.display = cardCategory === category ? '' : 'none'
        })
      })

      pill.addEventListener('keydown', event => {
        const pillList = Array.from(pills)
        moveFocusByArrowKey(event, pillList, pillList.indexOf(pill))
      })
    })
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
          moveFocusByArrowKey(event, buttons, buttons.indexOf(btn))
        })
      })
    })
  }

  const soccerLoginModal = document.getElementById('soccer-login-modal')
  let soccerLoginTrigger = null

  function setExpandedState(elements, isExpanded) {
    elements.forEach(element => {
      element.setAttribute('aria-expanded', isExpanded ? 'true' : 'false')
    })
  }

  function clearElements(elements) {
    elements.forEach(element => {
      element.replaceChildren()
    })
  }

  function closeAllSkillDetails() {
    setExpandedState(document.querySelectorAll('.skill-icon-btn[aria-expanded="true"]'), false)
    clearElements(document.querySelectorAll('.skill-detail-slot'))
  }

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

    if (soccerLoginTrigger && typeof soccerLoginTrigger.focus === 'function') {
      soccerLoginTrigger.focus()
      return
    }

    document.getElementById('maincontent')?.focus()
  }

  function resetSoccerResults() {
    const gamesContainer = document.getElementById('games-container')

    if (gamesContainer) {
      gamesContainer.replaceChildren()
    }
  }

  function setSoccerLoadingState(control, isLoading) {
    if (!control) {
      return
    }

    control.classList.toggle('is-loading', isLoading)

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

  // HTMX event handlers
  document.body.addEventListener('htmx:afterSwap', function (evt) {
    // Fade in new content
    if (evt.detail.target && !prefersReducedMotionQuery.matches) {
      evt.detail.target.classList.add('fade-in')
    }

    // Soccer page specific handlers - check for soccer form using data attribute
    if (evt.target.querySelector('[data-soccer-form]') || evt.target.id === 'games-container') {
      setupSoccerSelectAll()
    }

    if (evt.detail.target.id === 'soccer-login-feedback' && evt.detail.target.querySelector('[data-login-success]')) {
      const loginForm = document.getElementById('soccer-login-form')
      if (loginForm) {
        loginForm.reset()
      }
      closeSoccerLoginModal()
    }

    setupProjectsCategoryFilter()
    setupSkillsFilterKeyboard()

    // Skills page: set aria-expanded on the triggering skill button after detail loads
    if (evt.detail.elt && evt.detail.elt.classList.contains('skill-icon-btn') && evt.detail.target.classList.contains('skill-detail-slot')) {
      evt.detail.elt.setAttribute('aria-expanded', 'true')
    }

    // Skills page: re-observe new skill categories after filter swap
    if (evt.detail.target.id === 'skills-filterable' || evt.detail.target.closest('.skills-section')) {
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
    const loadingControl = getSoccerLoadingControl(evt.detail.elt)
    if (loadingControl) {
      setSoccerLoadingState(loadingControl, true)
    }

    if (evt.detail.elt && evt.detail.elt.classList.contains('skill-icon-btn')) {
      closeAllSkillDetails()
    }
  })

  ;['htmx:afterRequest', 'htmx:responseError', 'htmx:sendError'].forEach(eventName => {
    document.body.addEventListener(eventName, function (evt) {
      const loadingControl = getSoccerLoadingControl(evt.detail.elt)
      if (loadingControl) {
        setSoccerLoadingState(loadingControl, false)
      }
    })
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

      if (focusTarget) {
        focusTarget.focus()
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

    const loadingControl = getSoccerLoadingControl(event.submitter || event.target)
    if (!loadingControl || loadingControl.getAttribute('aria-busy') === 'true') {
      return
    }

    setSoccerLoadingState(loadingControl, true)
  })

  document.body.addEventListener('soccer-logout', resetSoccerResults)

  window.addEventListener('pageshow', resetSoccerLoadingLinks)

  // Initialize on page load (for non-HTMX scenarios)
  setupSoccerSelectAll()
  setupSoccerLoginModal()
  resetSoccerLoadingLinks()
  COUNTER_SECTIONS.forEach(({ section, duration, threshold }) => {
    observeCounterSection(section, '[data-counter]', duration, threshold)
  })
  setupProjectsCategoryFilter()
  setupSkillsFilterKeyboard()

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
    document.querySelectorAll('.timeline-item, .project-card, .skill-category').forEach(el => {
      observer?.observe(el)
    })
  }

  // Header scroll behavior
  const header = document.querySelector('.site-header')

  if (header) {
    let headerShadowVisible = false
    const syncHeaderShadow = () => {
      const shouldShowShadow = window.scrollY > HEADER_SCROLL_SHADOW_THRESHOLD
      if (shouldShowShadow === headerShadowVisible) {
        return
      }

      headerShadowVisible = shouldShowShadow
      header.style.boxShadow = shouldShowShadow ? 'var(--shadow-md)' : 'none'
    }

    window.addEventListener('scroll', syncHeaderShadow, { passive: true })
    syncHeaderShadow()
  }
})()
