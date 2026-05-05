/** @type {import('tailwindcss').Config} */
module.exports = {
  content: ["./internal/tmpl/**/*.html"],
  theme: {
    extend: {
      colors: {
        spot: {
          green: '#00754A',
          'green-dark': '#006241',
          'green-house': '#1E3932',
          'green-uplift': '#2b5148',
          'green-light': '#d4e9e2',
          bg: 'var(--spot-bg)',
          surface: 'var(--spot-surface)',
          hover: 'var(--spot-hover)',
          'hover-50': 'var(--spot-hover-50)',
          text: 'var(--spot-text)',
          secondary: 'var(--spot-secondary)',
          body: 'var(--spot-body)',
          muted: 'var(--spot-muted)',
          divider: 'var(--spot-divider)',
          'divider-30': 'var(--spot-divider-30)',
          outline: 'var(--spot-outline)',
          placeholder: 'var(--spot-placeholder)',
          'active-pill-bg': 'var(--spot-active-bg)',
          'active-pill-text': 'var(--spot-active-text)',
          red: '#c82014',
          orange: '#ffa42b',
          blue: '#539df5',
        }
      },
      borderRadius: {
        DEFAULT: '6px',
        sm: '6px',
        md: '8px',
        lg: '10px',
        xl: '12px',
        pill: '9999px',
        full: '50%',
      },
      boxShadow: {
        'spot': 'var(--spot-shadow)',
        'spot-heavy': 'var(--spot-shadow-heavy)',
        'spot-elevated': 'var(--spot-shadow-elevated)',
      },
      letterSpacing: {
        button: '1.4px',
      },
    },
  },
  plugins: [],
}
