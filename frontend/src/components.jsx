import { EQUIPMENTS, POUR_SPEEDS, TONE } from './labels'

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

// PhotoThumbs：客诉照片缩略图，点击新窗口放大。
export function PhotoThumbs({ photos }) {
  if (!photos || photos.length === 0) return <span className="muted">—</span>
  return (
    <span style={{ display: 'inline-flex', gap: 6, flexWrap: 'wrap' }}>
      {photos.map((p, i) => (
        <a key={i} href={p} target="_blank" rel="noreferrer">
          <img
            src={p}
            alt={`客诉照片${i + 1}`}
            style={{ width: 56, height: 38, objectFit: 'cover', borderRadius: 4, border: '1px solid var(--line)' }}
          />
        </a>
      ))}
    </span>
  )
}

// FeedbackFields：开桶反馈表单（出酒/气压/口味/客诉 + 出酒设备 + 客诉照片），BarOps 与 KegDetail 共用。
// fb 需包含 { pour_speed, pressure_issue, taste_issue, complaint, equipment, photos, detail }。
export function FeedbackFields({ fb, setFb }) {
  const addPhotos = (files) => {
    const room = 3 - (fb.photos?.length || 0)
    const list = Array.from(files || []).slice(0, room)
    list.forEach((f) => {
      if (f.size > 400 * 1024) {
        alert(`照片 ${f.name} 超过 400KB，请压缩后上传`)
        return
      }
      const reader = new FileReader()
      reader.onload = () => setFb((cur) => ({ ...cur, photos: [...(cur.photos || []), reader.result] }))
      reader.readAsDataURL(f)
    })
  }
  return (
    <>
      <Field label="出酒速度">
        <select value={fb.pour_speed} onChange={(e) => setFb({ ...fb, pour_speed: e.target.value })}>
          {POUR_SPEEDS.map((s) => <option key={s}>{s}</option>)}
        </select>
      </Field>
      <div className="field">
        <label style={{ display: 'flex', gap: 16 }}>
          <span><input type="checkbox" style={{ width: 'auto' }} checked={fb.pressure_issue} onChange={(e) => setFb({ ...fb, pressure_issue: e.target.checked })} /> 气压问题</span>
          <span><input type="checkbox" style={{ width: 'auto' }} checked={fb.taste_issue} onChange={(e) => setFb({ ...fb, taste_issue: e.target.checked })} /> 口味异常</span>
          <span><input type="checkbox" style={{ width: 'auto' }} checked={fb.complaint} onChange={(e) => setFb({ ...fb, complaint: e.target.checked })} /> 客诉</span>
        </label>
      </div>
      <Field label="出酒设备">
        <select value={fb.equipment} onChange={(e) => setFb({ ...fb, equipment: e.target.value })}>
          {EQUIPMENTS.map((s) => <option key={s}>{s}</option>)}
        </select>
      </Field>
      <Field label={`客诉照片（${(fb.photos || []).length}/3，单张 ≤400KB）`}>
        <input type="file" accept="image/*" multiple style={{ width: 'auto' }} onChange={(e) => { addPhotos(e.target.files); e.target.value = '' }} />
        {(fb.photos || []).length > 0 && (
          <div style={{ display: 'flex', gap: 8, marginTop: 8, flexWrap: 'wrap' }}>
            {fb.photos.map((p, i) => (
              <span key={i} style={{ position: 'relative', display: 'inline-block' }}>
                <img src={p} alt={`照片${i + 1}`} style={{ width: 72, height: 48, objectFit: 'cover', borderRadius: 4, border: '1px solid var(--line)' }} />
                <button
                  type="button"
                  className="small danger"
                  style={{ position: 'absolute', top: -6, right: -6, padding: '0 6px' }}
                  onClick={() => setFb({ ...fb, photos: fb.photos.filter((_, x) => x !== i) })}
                >
                  ×
                </button>
              </span>
            ))}
          </div>
        )}
      </Field>
      <Field label="详情">
        <textarea value={fb.detail} onChange={(e) => setFb({ ...fb, detail: e.target.value })} placeholder="描述出酒、气压、口味或客诉情况" />
      </Field>
    </>
  )
}
