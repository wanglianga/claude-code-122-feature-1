import { useCallback, useEffect, useState } from 'react'
import { api } from '../api'
import { Empty, ErrorBox, Field, Modal, OkBox, Tag } from '../components'
import { APPEARANCES, CAP_STATUSES, PICKUP_STATUS, RESIDUALS } from '../labels'

function PickupDetail({ id, user, onClose, onChanged }) {
  const [p, setP] = useState(null)
  const [error, setError] = useState('')
  const [ok, setOk] = useState('')
  const [scanItem, setScanItem] = useState(null)
  const [scan, setScan] = useState({ appearance: '完好', residual_liquid: '无', cap_status: '完好' })

  const load = useCallback(() => {
    api(`/pickups/${id}`).then(setP).catch((e) => setError(e.message))
  }, [id])
  useEffect(load, [load])

  const doScan = async () => {
    setError('')
    try {
      await api(`/pickups/${id}/items/${scanItem.id}/scan`, { method: 'POST', body: scan })
      setScanItem(null)
      setScan({ appearance: '完好', residual_liquid: '无', cap_status: '完好' })
      load()
      onChanged()
    } catch (e) {
      setError(e.message)
    }
  }

  const complete = async () => {
    setError('')
    setOk('')
    try {
      const res = await api(`/pickups/${id}/complete`, { method: 'POST' })
      setOk(res.missed && res.missed.length > 0 ? `已完成，漏扫 ${res.missed.length} 只已生成工单` : '回收完成，酒桶已送清洗厂')
      load()
      onChanged()
    } catch (e) {
      setError(e.message)
    }
  }

  if (!p) return <Modal title="回收单详情" onClose={onClose} wide><Empty text="加载中…" /></Modal>
  const isDriver = ['driver', 'admin'].includes(user.role)

  return (
    <Modal title={`回收单 ${p.pickup_no}`} onClose={onClose} wide>
      <ErrorBox error={error} onClose={() => setError('')} />
      <OkBox text={ok} onClose={() => setOk('')} />
      <div className="kv" style={{ marginBottom: 14 }}>
        <div><div className="k">酒吧</div><div className="v">{p.bar_name}</div></div>
        <div><div className="k">状态</div><div className="v"><Tag label={PICKUP_STATUS[p.status] || p.status} /></div></div>
        <div><div className="k">应收 / 已扫</div><div className="v">{p.expected_count} / {p.scanned_count}</div></div>
        <div><div className="k">预约时间</div><div className="v">{p.created_at}</div></div>
        {p.note && <div><div className="k">备注</div><div className="v">{p.note}</div></div>}
      </div>
      <table>
        <thead><tr><th>桶号</th><th>批次</th><th>容量</th><th>扫码</th><th>外观</th><th>残液</th><th>封盖</th><th></th></tr></thead>
        <tbody>
          {p.items.map((it) => (
            <tr key={it.id}>
              <td className="mono">{it.keg_no}</td>
              <td className="mono">{it.batch_no}</td>
              <td>{it.capacity_liters}L</td>
              <td>{it.scanned ? <Tag label="已扫" tone="green" /> : <Tag label="未扫" tone="amber" />}</td>
              <td>{it.appearance || '—'}</td>
              <td>{it.residual_liquid || '—'}</td>
              <td>{it.cap_status || '—'}</td>
              <td>
                {isDriver && p.status === 'booked' && !it.scanned && (
                  <button className="small primary" onClick={() => setScanItem(it)}>扫码</button>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      {isDriver && p.status === 'booked' && (
        <div className="actions">
          <span className="muted" style={{ marginRight: 'auto' }}>
            未扫码的桶将记为「司机漏扫」并生成工单
          </span>
          <button className="primary" onClick={complete}>完成回收</button>
        </div>
      )}

      {scanItem && (
        <Modal title={`扫码确认 · ${scanItem.keg_no}`} onClose={() => setScanItem(null)}>
          <Field label="桶体外观">
            <select value={scan.appearance} onChange={(e) => setScan({ ...scan, appearance: e.target.value })}>
              {APPEARANCES.map((s) => <option key={s}>{s}</option>)}
            </select>
          </Field>
          <Field label="残液情况">
            <select value={scan.residual_liquid} onChange={(e) => setScan({ ...scan, residual_liquid: e.target.value })}>
              {RESIDUALS.map((s) => <option key={s}>{s}</option>)}
            </select>
          </Field>
          <Field label="封盖状态">
            <select value={scan.cap_status} onChange={(e) => setScan({ ...scan, cap_status: e.target.value })}>
              {CAP_STATUSES.map((s) => <option key={s}>{s}</option>)}
            </select>
          </Field>
          <div className="actions">
            <button onClick={() => setScanItem(null)}>取消</button>
            <button className="primary" onClick={doScan}>确认扫码</button>
          </div>
        </Modal>
      )}
    </Modal>
  )
}

export default function Pickups({ user }) {
  const [list, setList] = useState([])
  const [error, setError] = useState('')
  const [detailId, setDetailId] = useState(null)

  const load = useCallback(() => {
    api('/pickups').then(setList).catch((e) => setError(e.message))
  }, [])
  useEffect(load, [load])

  return (
    <div>
      <h2 className="page-title">回收预约</h2>
      <p className="page-sub">酒吧预约 → 司机扫码确认空桶数量 / 外观 / 残液 / 封盖 → 送清洗厂</p>
      <ErrorBox error={error} onClose={() => setError('')} />
      <div className="card">
        {list.length === 0 ? <Empty text="暂无回收预约（酒吧可在「酒吧操作台」勾选空桶预约）" /> : (
          <table>
            <thead><tr><th>回收单号</th><th>酒吧</th><th>状态</th><th>应收/已扫</th><th>备注</th><th>预约时间</th><th></th></tr></thead>
            <tbody>
              {list.map((p) => (
                <tr key={p.id}>
                  <td className="mono">{p.pickup_no}</td>
                  <td>{p.bar_name}</td>
                  <td><Tag label={PICKUP_STATUS[p.status] || p.status} /></td>
                  <td>{p.expected_count} / {p.scanned_count}</td>
                  <td className="muted">{p.note || '—'}</td>
                  <td className="muted">{p.created_at}</td>
                  <td><button className="small" onClick={() => setDetailId(p.id)}>详情 / 扫码</button></td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
      {detailId && <PickupDetail id={detailId} user={user} onClose={() => setDetailId(null)} onChanged={load} />}
    </div>
  )
}
