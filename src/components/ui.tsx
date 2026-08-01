import { useEffect, useRef, useState } from 'preact/hooks'
import type { ChipState, Status } from '../types'

export function Dot({ status }: { status: Status }) {
  return <span class={`dot ${status}`} title={status} />
}

export function Chip({
  label,
  state,
  value,
}: {
  label: string
  state?: ChipState
  value?: string
}) {
  const [pos, setPos] = useState<{ x: number; y: number } | null>(null)
  const ref = useRef<HTMLSpanElement>(null)
  const hasValue = value !== undefined
  const show = () => {
    const el = ref.current
    if (!el) return
    const r = el.getBoundingClientRect()
    setPos({ x: r.left + r.width / 2, y: r.top })
  }
  const hide = () => setPos(null)
  return (
    <>
      <span
        ref={ref}
        class={`chip ${state ?? 'unknown'}`}
        title={hasValue ? undefined : `${label}: ${state}`}
        onMouseEnter={show}
        onMouseLeave={hide}
      >
        {label}
        {!hasValue && <span class="chip-dot" />}
      </span>
      {hasValue && pos && (
        <span
          class="chip-pop"
          style={{ left: `${pos.x}px`, top: `${pos.y}px` }}
        >
          {value}
        </span>
      )}
    </>
  )
}

export function CheckState({ state }: { state: ChipState }) {
  return (
    <span class="check-state">
      <span class={`check-dot ${state}`} />
      {state}
    </span>
  )
}

type Theme = 'system' | 'light' | 'dark'

const THEMES: Theme[] = ['system', 'light', 'dark']
const THEME_ICONS: Record<Theme, string> = {
  system: 'mdi:theme-light-dark',
  light: 'mdi:weather-sunny',
  dark: 'mdi:weather-night',
}
const STORAGE_KEY = 'statuspage-theme'

function applyTheme(theme: Theme) {
  const light = document.getElementById('theme-light') as HTMLLinkElement | null
  const dark = document.getElementById('theme-dark') as HTMLLinkElement | null
  if (!light || !dark) return
  if (theme === 'dark') {
    light.media = 'not all'
    dark.media = 'all'
  } else if (theme === 'light') {
    light.media = 'all'
    dark.media = 'not all'
  } else {
    light.media = '(prefers-color-scheme: light)'
    dark.media = '(prefers-color-scheme: dark)'
  }
}

function readTheme(): Theme {
  try {
    const v = localStorage.getItem(STORAGE_KEY)
    if (v === 'light' || v === 'dark' || v === 'system') return v
  } catch {
    /* ignore */
  }
  return 'system'
}

export function ThemeButton() {
  const [theme, setTheme] = useState<Theme>(() => {
    const t = readTheme()
    applyTheme(t)
    return t
  })

  useEffect(() => {
    applyTheme(theme)
    try {
      localStorage.setItem(STORAGE_KEY, theme)
    } catch {
      /* ignore */
    }
  }, [theme])

  const cycle = () => {
    const i = THEMES.indexOf(theme)
    setTheme(THEMES[(i + 1) % THEMES.length] ?? 'system')
  }

  return (
    <button
      type="button"
      class="theme-btn"
      title={`theme: ${theme}`}
      aria-label={`theme: ${theme}`}
      onClick={cycle}
    >
      <iconify-icon icon={THEME_ICONS[theme]} width="18" height="18" />
    </button>
  )
}
