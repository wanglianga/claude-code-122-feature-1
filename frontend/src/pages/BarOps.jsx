import { useCallback, useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api } from '../api'
import { Empty, ErrorBox, Field, Modal, OkBox, Tag } from '../components'
import { KEG_STATUS, POUR_SPEEDS } from '../labels'

// 酒吧操作台：本店酒桶的开桶 / 反馈 / 打空 / 预约回收。
export default function BarOps({ user }) {
  const [kegs, setKegs] = useState([])
  const [error, setError] = useState('')
  const [ok, setOk] = useState('')
  const [fbKeg, setFbKeg] = useState(null)
  const [fb, setFb] = useState({ pour_speed: '正常', pressure_issue: false, taste_issue: false, complaint: false, detail: '' })
  const [selected, setSelected] = useState([])
  const [showPickup, setShowPickup] = useState(false)
  const [pickupNote, setPickupNote] = useState('')
  const navigate = useNavigate()

  const load = useCallback(() => {
    api('/kegs').then(setKegs).catch((e) => setError(e.message))
  }, [])
  useEffect(load, [load])

  const act = async (path, body) => {
    setError('')
    setOk('')
    try {
      await api(path, { method: 'POST', body })
      setOk('操作成功')
      setFbKeg(null)
      setShowPickup(false)
      setSelected([])
      setPickupNote('')
      load()
    } catch (e) {
      setError(e.message)
    }
  }

  const groups = [
    ['at_bar', '在店未开桶'],
    ['opened', '已开桶'],
    ['empty', '已打空（可预约回收）'],
    ['pickup_booked', '已预约回收（待司机扫码）'],
  ]
  const recyclable = kegs.filter((k) => ['empty', 'at_bar'].includes(k.status))
  const toggle = (id) => setSelected((s) => (s.includes(id) ? s.filter((x) => x !== id) : [...s, id]))

  return (
    <div>
      <h2 className="page-title">酒吧操作台</h2>
      <p className="page-sub">本店酒桶的开桶、反馈、打空与回收预约</p>
      <ErrorBox error={error} onClose={() => setError('')} />
      <OkBox text={ok} onClose={() => setOk('')} />

      <div className="toolbar">
        <span className="muted">勾选「已打空 / 未开桶」酒桶可预约回收：</span>
        <div className="spacer" />
        <button
          className="primary"
          disabled={selected.length === 0}
          onClick={() => setShowPickup(true)}
        >
          预约回收（已选 {selected.length} 只）
        </button>
      </div>

      {groups.map(([status, title]) => {
        const list = kegs.filter((k) => k.status === status)
        if (list.length === 0) return null
        return (
          <div className="card" key={status}>
            <h3>{title}（{list.length}）</h3>
            <table>
              <thead>
                <tr><th></th><th>桶号</th><th>批次</th><th>酒款</th><th>容量</th><th>押金</th><th>操作</th></tr>
              </thead>
              <tbody>
                {list.map((k) => (
                  <tr key={k.id}>
                    <td style={{ width: 30 }}>
                      {['empty', 'at_bar'].includes(k.status) && (
                        <input type="checkbox" style={{ width: 'auto' }} checked={selected.includes(k.id)} onChange={() => toggle(k.id)} />
                      )}
                    </td>
                    <td className="mono" style={{ cursor: 'pointer' }} onClick={() => navigate(`/kegs/${k.id}`)}>{k.keg_no}</td>
                    <td className="mono">{k.batch_no}</td>
                    <td>{k.style}</td>
                    <td>{k.capacity_liters}L</td>
                    <td>¥{k.deposit_amount}</td>
                    <td style={{ whiteSpace: 'nowrap' }}>
                      {k.status === 'at_bar' && (
                        <button className="small primary" onClick={() => act(`/kegs/${k.id}/open`)}>开桶</button>
                      )}{' '}
                      {['at_bar', 'opened'].includes(k.status) && (
                        <button className="small" onClick={() => { setFbKeg(k); setFb({ pour_speed: '正常', pressure_issue: false, taste_issue: false, complaint: false, detail: '' }) }}>反馈</button>
                      )}{' '}
                      {k.status === 'opened' && (
                        <button className="small" onClick={() => act(`/kegs/${k.id}/empty`)}>打空</button>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )
      })}
      {kegs.length === 0 && <div className="card"><Empty text="本店暂无酒桶" /></div>}

      {fbKeg && (
        <Modal title={`开桶反馈 · ${fbKeg.keg_no}`} onClose={() => setFbKeg(null)}>
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
          <Field label="详情">
            <textarea value={fb.detail} onChange={(e) => setFb({ ...fb, detail: e.target.value })} />
          </Field>
          <div className="actions">
            <button onClick={() => setFbKeg(null)}>取消</button>
            <button className="primary" onClick={() => act(`/kegs/${fbKeg.id}/feedback`, fb)}>提交</button>
          </div>
        </Modal>
      )}

      {showPickup && (
        <Modal title={`预约回收（${selected.length} 只）`} onClose={() => setShowPickup(false)}>
          <div className="chips" style={{ marginBottom: 12 }}>
            {recyclable.filter((k) => selected.includes(k.id)).map((k) => (
              <span key={k.id} className="tag mono">{k.keg_no}</span>
            ))}
          </div>
          <Field label="备注">
            <input value={pickupNote} onChange={(e) => setPickupNote(e.target.value)} placeholder="如：下午 3 点后在店，后门卸货区" />
          </Field>
          <div className="actions">
            <button onClick={() => setShowPickup(false)}>取消</button>
            <button className="primary" onClick={() => act('/pickups', { keg_ids: selected, note: pickupNote })}>提交预约</button>
          </div>
        </Modal>
      )}
    </div>
  )
}
