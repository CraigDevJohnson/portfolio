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

  // Initialize on page load (for non-HTMX scenarios)
  setupEmailSubscription()
  setupSoccerSelectAll()

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
