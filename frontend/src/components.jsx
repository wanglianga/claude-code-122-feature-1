import { useState } from 'react'
import { uploadFile } from './api'
import { EQUIPMENTS, POUR_SPEEDS, TASTE_DESCS, TONE } from './labels'

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

// PhotoUploader 客诉照片上传（演示环境存本地，通过 /uploads 访问）。
export function PhotoUploader({ photos, onChange, onError }) {
  const [uploading, setUploading] = useState(false)
  const pick = async (files) => {
    for (const file of Array.from(files || [])) {
      setUploading(true)
      try {
        const r = await uploadFile(file)
        onChange([...photos, { name: r.name, url: r.url }])
      } catch (e) {
        onError?.(e.message)
      } finally {
        setUploading(false)
      }
    }
  }
  return (
    <div>
      <div className="chips" style={{ marginBottom: 8 }}>
        {photos.map((p) => (
          <span key={p.name} className="photo-chip">
            <a href={p.url} target="_blank" rel="noreferrer">
              <img src={p.url} alt="客诉照片" />
            </a>
            <button type="button" className="photo-del" onClick={() => onChange(photos.filter((x) => x.name !== p.name))}>×</button>
          </span>
        ))}
        <label className="photo-add">
          <input type="file" accept="image/*" multiple style={{ display: 'none' }} onChange={(e) => pick(e.target.files)} />
          {uploading ? '上传中…' : '＋ 客诉照片'}
        </label>
      </div>
    </div>
  )
}

// FeedbackModal 统一的开桶反馈弹窗：出酒/气压/口味现象/出酒设备/客诉照片。
export function FeedbackModal({ kegNo, onClose, onSubmit }) {
  const [fb, setFb] = useState({
    pour_speed: '正常', pressure_issue: false, taste_issue: false, complaint: false,
    equipment: EQUIPMENTS[0], taste_desc: '', detail: '',
  })
  const [photos, setPhotos] = useState([])
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)
  const taste = fb.taste_issue || fb.complaint

  const submit = async () => {
    setError('')
    setSaving(true)
    try {
      await onSubmit({ ...fb, taste_desc: taste ? fb.taste_desc : '', photos: photos.map((p) => p.name) })
    } catch (e) {
      setError(e.message)
    } finally {
      setSaving(false)
    }
  }

  return (
    <Modal title={`开桶反馈 · ${kegNo}`} onClose={onClose}>
      <ErrorBox error={error} onClose={() => setError('')} />
      <Field label="出酒速度">
        <select value={fb.pour_speed} onChange={(e) => setFb({ ...fb, pour_speed: e.target.value })}>
          {POUR_SPEEDS.map((s) => <option key={s}>{s}</option>)}
        </select>
      </Field>
      <Field label="出酒设备">
        <select value={fb.equipment} onChange={(e) => setFb({ ...fb, equipment: e.target.value })}>
          {EQUIPMENTS.map((s) => <option key={s}>{s}</option>)}
        </select>
      </Field>
      <div className="field">
        <label style={{ display: 'flex', gap: 16 }}>
          <span><input type="checkbox" style={{ width: 'auto' }} checked={fb.pressure_issue} onChange={(e) => setFb({ ...fb, pressure_issue: e.target.checked })} /> 气压问题</span>
          <span><input type="checkbox" style={{ width: 'auto' }} checked={fb.taste_issue} onChange={(e) => setFb({ ...fb, taste_issue: e.target.checked })} /> 口味异常</span>
          <span><input type="checkbox" style={{ width: 'auto' }} checked={fb.complaint} onChange={(e) => setFb({ ...fb, complaint: e.target.checked })} /> 客诉</span>
        </label>
      </div>
      {taste && (
        <Field label="口味现象（供品控关联分析）">
          <select value={fb.taste_desc} onChange={(e) => setFb({ ...fb, taste_desc: e.target.value })}>
            <option value="">请选择</option>
            {TASTE_DESCS.map((s) => <option key={s}>{s}</option>)}
          </select>
        </Field>
      )}
      {taste && (
        <Field label="客诉照片">
          <PhotoUploader photos={photos} onChange={setPhotos} onError={setError} />
        </Field>
      )}
      <Field label="详情">
        <textarea value={fb.detail} onChange={(e) => setFb({ ...fb, detail: e.target.value })}
          placeholder="如：开桶 3 天后出现纸板味，两桌客人退酒；请记录出酒设备清洗情况" />
      </Field>
      <div className="actions">
        <button onClick={onClose}>取消</button>
        <button className="primary" disabled={saving} onClick={submit}>{saving ? '提交中…' : '提交反馈'}</button>
      </div>
    </Modal>
  )
}
