import { TONE } from './labels'

// Tag：带颜色的状态标签，颜色由 TONE 映射决定，也可显式指定。
export function Tag({ label, tone }) {
  return <span className={`badge t-${tone || TONE[label] || 'gray'}`}>{label}</span>
}

export function Modal({ title, onClose, children, wide }) {
  return (
    <div className="modal-mask" onClick={onClose}>
      <div className={`modal${wide ? ' wide' : ''}`} onClick={(e) => e.stopPropagation()}>
        <h3>{title}</h3>
        {children}
      </div>
    </div>
  )
}

export function Field({ label, children }) {
  return (
    <div className="field">
      <label>{label}</label>
      {children}
    </div>
  )
}

export function Empty({ text }) {
  return <div className="muted" style={{ padding: '18px 0', textAlign: 'center' }}>{text || '暂无数据'}</div>
}

export function ErrorBox({ error, onClose }) {
  if (!error) return null
  return (
    <div className="error-box" onClick={onClose} title="点击关闭">
      {error}
    </div>
  )
}

export function OkBox({ text, onClose }) {
  if (!text) return null
  return (
    <div className="ok-box" onClick={onClose} title="点击关闭">
      {text}
    </div>
  )
}

export function KV({ items }) {
  return (
    <div className="kv">
      {items.map(([k, v]) => (
        <div key={k}>
          <div className="k">{k}</div>
          <div className="v">{v ?? '-'}</div>
        </div>
      ))}
    </div>
  )
}
