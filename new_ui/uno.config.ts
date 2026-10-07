import { defineConfig, presetUno } from 'unocss'

const v = (n: string) => `var(--${n})`
export default defineConfig({
  presets: [presetUno()],
  theme: {
    colors: {
      body: v('color-body'), primary: v('color-primary'), secondary: v('color-secondary'),
      tertiary: v('color-tertiary'), fill: v('color-fill'), selected: v('color-selected'),
      action: { DEFAULT: v('color-action'), hover: v('color-action-hover'), pressed: v('color-action-pressed') },
      disabled: v('color-disabled'),
      border: { default: v('color-border-default'), strong: v('color-border-strong') },
    },
    spacing: { xs: '6px', sm: '10px', md: '20px', lg: '30px' },
    fontSize: {
      sm: ['12px', '1.6'], md: ['14px', '1.6'], lg: ['16px', '1.6'], xl: ['18px', '1.6'],
      '2xl': ['20px', '1.6'], '3xl': ['22px', '1.6'], '4xl': ['24px', '1.6'], '8xl': ['32px', '1.25'],
    },
  },
  rules: [
    // 文字色用 text-primary / text-secondary …，与背景色名区分开
    [/^text-(primary|secondary|tertiary|quaternary|info|success|warning|error|keyword)$/, ([, c]) => ({ color: `var(--color-text-${c})` })],
  ],
  shortcuts: {
    'ax-box': 'rounded-16px p-sm relative bg-primary ring-1 ring-inset ring-[var(--color-border-strong)]',
    'ax-filter-bar': 'flex items-center justify-between gap-sm h-40px rounded-10px px-16px py-3px bg-fill',
    'ax-row': 'bg-fill! border border-solid border-transparent transition-colors duration-200 hover:border-action! cursor-pointer',
  },
})
