/** @type {import('tailwindcss').Config} */
module.exports = {
  content: ["./internal/tmpl/**/*.html"],
  theme: {
    extend: {
      colors: {
        spot: {
          purple: '#a855f7',
          'purple-border': '#9333ea',
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
          red: '#f3727f',
          orange: '#ffa42b',
          blue: '#539df5',
        }
      },
      fontFamily: {
        ui: ['SpotifyMixUI', 'CircularSp-Arab', 'CircularSp-Hebr', 'CircularSp-Cyrl', 'CircularSp-Grek', 'CircularSp-Deva', 'Helvetica Neue', 'helvetica', 'arial', 'Hiragino Sans', 'Hiragino Kaku Gothic ProN', 'Meiryo', 'MS Gothic', 'sans-serif'],
        title: ['SpotifyMixUITitle', 'CircularSp-Arab', 'CircularSp-Hebr', 'CircularSp-Cyrl', 'CircularSp-Grek', 'CircularSp-Deva', 'Helvetica Neue', 'helvetica', 'arial', 'Hiragino Sans', 'Hiragino Kaku Gothic ProN', 'Meiryo', 'MS Gothic', 'sans-serif'],
      },
      borderRadius: {
        pill: '9999px',
        'pill-lg': '500px',
      },
      boxShadow: {
        'spot': 'var(--spot-shadow)',
        'spot-heavy': 'var(--spot-shadow-heavy)',
      },
      letterSpacing: {
        button: '1.4px',
      },
    },
  },
  plugins: [],
}
