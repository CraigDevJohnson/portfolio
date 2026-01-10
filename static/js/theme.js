// Theme toggle functionality
;(function () {
  const root = document.documentElement
  const themeToggle = document.getElementById('theme-toggle')

  // Check for saved theme preference or default to system preference
  const savedTheme = localStorage.getItem('theme')
  if (savedTheme) {
    root.classList.toggle('dark', savedTheme === 'dark')
  } else {
    // Default to system preference when no saved theme exists
    const prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches
    root.classList.toggle('dark', prefersDark)
  }

  // Theme toggle click handler
  if (themeToggle) {
    themeToggle.addEventListener('click', () => {
      root.classList.toggle('dark')
      const isDark = root.classList.contains('dark')
      localStorage.setItem('theme', isDark ? 'dark' : 'light')
    })
  }

  // Listen for system theme changes
  window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', e => {
    if (!localStorage.getItem('theme')) {
      root.classList.toggle('dark', e.matches)
    }
  })
})()
