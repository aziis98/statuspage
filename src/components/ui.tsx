import { useRef, useState } from 'preact/hooks'
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
