// Main JavaScript functionality
;(function () {
  'use strict'

  // Mobile menu toggle
  const mobileMenuBtn = document.getElementById('mobile-menu-btn')
  const mobileNav = document.getElementById('mobile-nav')

  if (mobileMenuBtn && mobileNav) {
    mobileMenuBtn.addEventListener('click', () => {
      const isExpanded = mobileMenuBtn.getAttribute('aria-expanded') === 'true'
      mobileMenuBtn.setAttribute('aria-expanded', !isExpanded)
      mobileNav.classList.toggle('open')
      mobileNav.setAttribute('aria-hidden', isExpanded)
    })

    // Close menu when clicking on a link
    mobileNav.querySelectorAll('.nav-link').forEach(link => {
      link.addEventListener('click', () => {
        mobileMenuBtn.setAttribute('aria-expanded', 'false')
        mobileNav.classList.remove('open')
        mobileNav.setAttribute('aria-hidden', 'true')
      })
    })

    // Close menu when clicking outside
    document.addEventListener('click', e => {
      if (!mobileMenuBtn.contains(e.target) && !mobileNav.contains(e.target)) {
        mobileMenuBtn.setAttribute('aria-expanded', 'false')
        mobileNav.classList.remove('open')
        mobileNav.setAttribute('aria-hidden', 'true')
      }
    })
  }

  // Smooth scroll for anchor links
  document.querySelectorAll('a[href^="#"]').forEach(anchor => {
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
    const firstField = soccerLoginModal.querySelector('#soccer-login-email')
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
      gamesContainer.innerHTML = '<div class="empty-state"><p>Sign in to fetch by player, or enter team codes above to fetch schedules.</p></div>'
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

  function setupSoccerLoginForm() {
    const loginForm = document.getElementById('soccer-login-form')
    if (!loginForm || loginForm.dataset.bound === 'true') {
      return
    }

    loginForm.dataset.bound = 'true'
    const captchaTokenField = loginForm.querySelector('input[name="captcha_token"]')
    const feedback = document.getElementById('soccer-login-feedback')

    loginForm.addEventListener('submit', event => {
      const siteKey = loginForm.dataset.recaptchaSiteKey

      if (!siteKey || !captchaTokenField) {
        return
      }

      if (captchaTokenField.value) {
        return
      }

      if (!window.grecaptcha) {
        event.preventDefault()
        if (feedback) {
          feedback.innerHTML = '<div class="soccer-login-message soccer-login-message-error" role="alert">reCAPTCHA is still loading. Try again in a moment.</div>'
        }
        return
      }

      event.preventDefault()
      window.grecaptcha.ready(() => {
        window.grecaptcha
          .execute(siteKey, { action: 'soccer_login' })
          .then(token => {
            captchaTokenField.value = token
            if (window.htmx) {
              window.htmx.trigger(loginForm, 'submit')
            } else {
              loginForm.requestSubmit()
            }
          })
          .catch(() => {
            if (feedback) {
              feedback.innerHTML = '<div class="soccer-login-message soccer-login-message-error" role="alert">Could not start reCAPTCHA. Try again.</div>'
            }
          })
      })
    })

    document.body.addEventListener('htmx:afterRequest', event => {
      if (event.detail.elt === loginForm && captchaTokenField) {
        captchaTokenField.value = ''
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
    if (evt.detail.elt && evt.detail.elt.classList.contains('skill-icon-btn')) {
      document.querySelectorAll('.skill-detail-slot').forEach(function (slot) {
        slot.innerHTML = ''
      })
    }
  })

  document.body.addEventListener('soccer-logout', resetSoccerResults)

  // Initialize on page load (for non-HTMX scenarios)
  setupEmailSubscription()
  setupSoccerSelectAll()
  setupSoccerLoginModal()
  setupSoccerLoginForm()

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

      if (currentScroll > 100) {
        header.style.boxShadow = 'var(--shadow-md)'
      } else {
        header.style.boxShadow = 'none'
      }
    })
  }
})()
