import type { Machine } from '../types'
import { formatTime } from '../format'
import { parseScriptChips } from '../parse'
import { Chip, Dot } from './ui'

export function MachineCard({
  m,
  onOpen,
  interactive,
  onRefresh,
}: {
  m: Machine
  onOpen: (m: Machine) => void
  interactive: boolean
  onRefresh: (m: Machine) => void
}) {
  const sshState = !m.sshConfigured
    ? 'off'
    : !m.ssh
      ? 'unknown'
      : m.ssh.ok
        ? 'ok'
        : 'fail'
  const scriptChips = m.ssh && !m.ssh.error ? parseScriptChips(m.ssh.result) : []
  return (
    <div
      class="machine"
      data-status={m.status}
      onClick={() => onOpen(m)}
      role="button"
      tabIndex={0}
    >
      <div class="machine-head">
        <Dot status={m.status} />
        <span class="machine-name">{m.name}</span>
        {interactive && (
          <button
            class="refresh-btn"
            title="Refresh now"
            onClick={(e) => {
              e.stopPropagation()
              onRefresh(m)
            }}
          >
            <iconify-icon icon="mdi:refresh" width="15" height="15" />
          </button>
        )}
      </div>
      <div class="machine-ip">{m.ip || '—'}</div>
      <div class="chips">
        <Chip label="ping" state={m.icmp} />
        <Chip label="tcp" state={m.tcp} />
        {m.sshConfigured && <Chip label="ssh" state={sshState} />}
      </div>

      <div class="popover" onClick={(e) => e.stopPropagation()}>
        <div class="popover-head">
          <div class="popover-meta">
            <span class="popover-meta-item">
              <span class="popover-meta-key">ip</span>
              <span class="popover-meta-val">{m.ip || 'unknown'}</span>
            </span>
            <span class="popover-meta-item">
              <span class="popover-meta-key">last ping</span>
              <span class="popover-meta-val">{formatTime(m.lastPing)}</span>
            </span>
          </div>
          <span class="popover-status">
            <Dot status={m.status} />
            <span class={`badge ${m.status}`}>{m.status}</span>
          </span>
        </div>
        {scriptChips.length > 0 && (
          <table class="popover-table">
            <tbody>
              {scriptChips.map((c) => (
                <tr key={c.name}>
                  <th>{c.name}</th>
                  <td>
                    {c.status === 'unknown' ? 'unknown' : c.info || c.status}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  )
}

export function Grid({
  machines,
  onOpen,
  interactive,
  onRefresh,
}: {
  machines: Machine[]
  onOpen: (m: Machine) => void
  interactive: boolean
  onRefresh: (m: Machine) => void
}) {
  return (
    <div class="grid">
      {machines.map((m) => (
        <MachineCard
          key={m.id}
          m={m}
          onOpen={onOpen}
          interactive={interactive}
          onRefresh={onRefresh}
        />
      ))}
    </div>
  )
}
