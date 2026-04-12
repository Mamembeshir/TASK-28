/** @type {import('tailwindcss').Config} */
module.exports = {
  content: [
    "./internal/templ/**/*.templ",
    "./internal/templ/**/*_templ.go",
  ],
  theme: {
    extend: {
      fontFamily: {
        sans: ['"Source Sans 3"', 'sans-serif'],
        mono: ['"Source Code Pro"', 'monospace'],
      },
      colors: {
        surface: {
          primary: 'var(--surface-primary)',
          secondary: 'var(--surface-secondary)',
          elevated: 'var(--surface-elevated)',
          sunken: 'var(--surface-sunken)',
        },
        brand: {
          primary: 'var(--brand-primary)',
          'primary-hover': 'var(--brand-primary-hover)',
          'primary-light': 'var(--brand-primary-light)',
          accent: 'var(--brand-accent)',
        },
        success: {
          DEFAULT: 'var(--success)',
          light: 'var(--success-light)',
        },
        warning: {
          DEFAULT: 'var(--warning)',
          light: 'var(--warning-light)',
        },
        danger: {
          DEFAULT: 'var(--danger)',
          light: 'var(--danger-light)',
        },
        info: {
          DEFAULT: 'var(--info)',
          light: 'var(--info-light)',
        },
        text: {
          primary: 'var(--text-primary)',
          secondary: 'var(--text-secondary)',
          tertiary: 'var(--text-tertiary)',
          inverse: 'var(--text-inverse)',
        },
        border: {
          default: 'var(--border-default)',
          strong: 'var(--border-strong)',
          focus: 'var(--border-focus)',
        },
        tier: {
          bronze: 'var(--tier-bronze)',
          silver: 'var(--tier-silver)',
          gold: 'var(--tier-gold)',
        },
      },
      boxShadow: {
        sm: 'var(--shadow-sm)',
        md: 'var(--shadow-md)',
        lg: 'var(--shadow-lg)',
        focus: 'var(--shadow-focus)',
      },
      borderRadius: {
        card: '10px',
        btn: '6px',
        modal: '14px',
        pill: '9999px',
      },
      maxWidth: {
        content: '1200px',
      },
      spacing: {
        page: '28px',
        'page-mobile': '14px',
        section: '28px',
      },
      width: {
        sidebar: '250px',
      },
    },
  },
  plugins: [],
}
