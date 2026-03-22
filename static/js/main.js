// Main JavaScript functionality
;(function () {
  'use strict'

  const HEADER_SCROLL_SHADOW_THRESHOLD = 100

  // Mobile menu toggle
  const mobileMenuBtn = document.getElementById('mobile-menu-btn')
  const mobileNav = document.getElementById('mobile-nav')

  if (mobileMenuBtn && mobileNav) {
    const closeMobileNav = () => {
      mobileMenuBtn.setAttribute('aria-expanded', 'false')
      mobileNav.classList.remove('open')
      mobileNav.setAttribute('aria-hidden', 'true')
    }

    mobileMenuBtn.addEventListener('click', () => {
      const isExpanded = mobileMenuBtn.getAttribute('aria-expanded') === 'true'
      mobileMenuBtn.setAttribute('aria-expanded', !isExpanded)
      mobileNav.classList.toggle('open')
      mobileNav.setAttribute('aria-hidden', isExpanded)
    })

    // Close menu when clicking on a link
    mobileNav.querySelectorAll('.nav-link').forEach(link => {
      link.addEventListener('click', () => {
        closeMobileNav()
      })
    })

    // Close menu when clicking outside
    document.addEventListener('click', e => {
      if (!mobileMenuBtn.contains(e.target) && !mobileNav.contains(e.target)) {
        closeMobileNav()
      }
    })
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
    const selectAll = document.querySelector('[data-select-all]')
    const gameCheckboxes = document.querySelectorAll('[data-game-checkbox]')

    if (selectAll && gameCheckboxes.length > 0) {
      // Select all toggle
      selectAll.addEventListener('change', function () {
        gameCheckboxes.forEach(cb => {
          cb.checked = selectAll.checked
        })
      })

      // Update select all state based on individual checkboxes
      gameCheckboxes.forEach(cb => {
        cb.addEventListener('change', function () {
          const allChecked =
            document.querySelectorAll('[data-game-checkbox]:checked').length ===
            gameCheckboxes.length
          selectAll.checked = allChecked
        })
      })
    }
  }

  // Email subscription toggle
  function setupEmailSubscription() {
    const emailCheckbox = document.getElementById('email-updates-checkbox')
    const subscribeForm = document.getElementById('subscribe-form')

    if (emailCheckbox && subscribeForm) {
      emailCheckbox.addEventListener('change', () => {
        subscribeForm.style.display = emailCheckbox.checked ? 'block' : 'none'
        if (emailCheckbox.checked) {
          const emailInput = document.getElementById('subscription-email')
          if (emailInput) emailInput.focus()
        }
      })
    }
  }

  // Show subscribe section after games load
  function showSubscribeSection() {
    const subscribeSection = document.getElementById('subscribe-section')
    if (subscribeSection) {
      subscribeSection.style.display = 'block'
      subscribeSection.classList.add('fade-in')
    }
  }

  function animateCounter(counter, duration) {
    const targetYear = counter.dataset.targetYear
    const targetValue = counter.dataset.target
    const suffix = counter.dataset.suffix || ''
    const finalValue = targetYear
      ? new Date().getFullYear() - Number.parseInt(targetYear, 10)
      : Number.parseInt(targetValue, 10) || 0

    const start = performance.now()

    function update(now) {
      const elapsed = now - start
      const progress = Math.min(elapsed / duration, 1)
      const ease = 1 - Math.pow(1 - progress, 3)
      counter.textContent = Math.floor(finalValue * ease)

      if (progress < 1) {
        requestAnimationFrame(update)
        return
      }

      counter.textContent = `${finalValue}${suffix}`
    }

    requestAnimationFrame(update)
  }

  function observeCounterSection(sectionSelector, counterSelector, duration) {
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
      { threshold: 0.3 }
    )

    observer.observe(section)
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
    })
  }

  const soccerLoginModal = document.getElementById('soccer-login-modal')
  let soccerLoginTrigger = null

  function getModalFocusableElements() {
    if (!soccerLoginModal) {
      return []
    }

    return Array.from(
      soccerLoginModal.querySelectorAll(
        'button:not([disabled]), input:not([disabled]), a[href], select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'
      )
    ).filter(element => !element.hidden)
  }

  function openSoccerLoginModal(trigger) {
    if (!soccerLoginModal) {
      return
    }

    soccerLoginTrigger = trigger || document.activeElement
    soccerLoginModal.hidden = false
    document.body.classList.add('soccer-modal-open')

    const dialog = soccerLoginModal.querySelector('.soccer-login-dialog')
    const firstField = soccerLoginModal.querySelector('#soccer-import-jwt')
    window.setTimeout(() => {
      ;(firstField || dialog)?.focus()
    }, 0)
  }

  function closeSoccerLoginModal() {
    if (!soccerLoginModal) {
      return
    }

    soccerLoginModal.hidden = true
    document.body.classList.remove('soccer-modal-open')

    if (soccerLoginTrigger && typeof soccerLoginTrigger.focus === 'function') {
      soccerLoginTrigger.focus()
    }
  }

  function resetSoccerResults() {
    const gamesContainer = document.getElementById('games-container')
    const subscribeSection = document.getElementById('subscribe-section')
    const subscribeForm = document.getElementById('subscribe-form')
    const emailCheckbox = document.getElementById('email-updates-checkbox')
    const subscribeResult = document.getElementById('subscribe-result')

    if (gamesContainer) {
      gamesContainer.innerHTML = '<div class="empty-state"><p>Import player access or enter team codes above to fetch schedules.</p></div>'
    }

    if (subscribeSection) {
      subscribeSection.style.display = 'none'
    }

    if (emailCheckbox) {
      emailCheckbox.checked = false
    }

    if (subscribeForm) {
      subscribeForm.style.display = 'none'
    }

    if (subscribeResult) {
      subscribeResult.innerHTML = ''
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
    if (evt.detail.target) {
      evt.detail.target.classList.add('fade-in')
    }

    // Soccer page specific handlers - check for soccer form using data attribute
    if (evt.target.querySelector('[data-soccer-form]') || evt.target.id === 'games-container') {
      showSubscribeSection()
      setupSoccerSelectAll()
      setupEmailSubscription()
    }

    if (evt.detail.target.id === 'soccer-login-feedback' && evt.detail.target.querySelector('[data-login-success]')) {
      const loginForm = document.getElementById('soccer-login-form')
      if (loginForm) {
        loginForm.reset()
      }
      closeSoccerLoginModal()
    }

    setupProjectsCategoryFilter()

    // Skills page: re-observe new skill categories after filter swap
    if (evt.detail.target.id === 'skills-filterable' || evt.detail.target.closest('.skills-section')) {
      const newCategories = evt.detail.target.querySelectorAll('.skill-category')
      newCategories.forEach(function (el) {
        observer.observe(el)
      })
    }
  })

  // Skills page: close all detail panels before opening a new one
  document.body.addEventListener('htmx:beforeRequest', function (evt) {
    const loadingControl = getSoccerLoadingControl(evt.detail.elt)
    if (loadingControl) {
      setSoccerLoadingState(loadingControl, true)
    }

    if (evt.detail.elt && evt.detail.elt.classList.contains('skill-icon-btn')) {
      document.querySelectorAll('.skill-detail-slot').forEach(function (slot) {
        slot.innerHTML = ''
      })
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

  document.body.addEventListener('soccer-logout', resetSoccerResults)

  window.addEventListener('pageshow', resetSoccerLoadingLinks)

  // Initialize on page load (for non-HTMX scenarios)
  setupEmailSubscription()
  setupSoccerSelectAll()
  setupSoccerLoginModal()
  resetSoccerLoadingLinks()
  observeCounterSection('.hero-stats', '.home-stat-value', 2000)
  observeCounterSection('.about-stats', '.about-stat-value', 2000)
  observeCounterSection('.edu-stats', '.edu-stat-value[data-target]', 1500)
  observeCounterSection('.projects-stats', '.proj-stat-value', 1500)
  setupProjectsCategoryFilter()

  // Add intersection observer for scroll animations
  const observerOptions = {
    root: null,
    rootMargin: '0px',
    threshold: 0.1,
  }

  const observer = new IntersectionObserver(entries => {
    entries.forEach(entry => {
      if (entry.isIntersecting) {
        entry.target.classList.add('fade-in')
        observer.unobserve(entry.target)
      }
    })
  }, observerOptions)

  // Observe elements that should animate on scroll
  document.querySelectorAll('.timeline-item, .project-card, .skill-category').forEach(el => {
    observer.observe(el)
  })

  // Header scroll behavior
  const header = document.querySelector('.site-header')

  if (header) {
    window.addEventListener('scroll', () => {
      const currentScroll = window.pageYOffset

      if (currentScroll > HEADER_SCROLL_SHADOW_THRESHOLD) {
        header.style.boxShadow = 'var(--shadow-md)'
      } else {
        header.style.boxShadow = 'none'
      }
    })
  }
})()
