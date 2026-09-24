import { useCallback, useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api } from '../api'
import { Empty, ErrorBox, Field, Modal, Tag } from '../components'
import { KEG_STATUS } from '../labels'

export default function Kegs({ user }) {
  const [list, setList] = useState([])
  const [meta, setMeta] = useState({ batches: [] })
  const [filters, setFilters] = useState({ status: '', batch_id: '', q: '' })
  const [error, setError] = useState('')
  const [showCreate, setShowCreate] = useState(false)
  const [form, setForm] = useState({ keg_no: '', capacity_liters: 50, batch_id: '', deposit_amount: 300 })
  const navigate = useNavigate()

  useEffect(() => {
    api('/meta').then(setMeta).catch(() => {})
  }, [])

  const load = useCallback(() => {
    const qs = new URLSearchParams(Object.entries(filters).filter(([, v]) => v !== '')).toString()
    api('/kegs' + (qs ? '?' + qs : '')).then(setList).catch((e) => setError(e.message))
  }, [filters])
  useEffect(load, [load])

  const create = async () => {
    setError('')
    try {
      await api('/kegs', {
        method: 'POST',
        body: { ...form, capacity_liters: Number(form.capacity_liters), batch_id: Number(form.batch_id), deposit_amount: Number(form.deposit_amount) },
      })
      setShowCreate(false)
      setForm({ keg_no: '', capacity_liters: 50, batch_id: '', deposit_amount: 300 })
      load()
    } catch (e) {
      setError(e.message)
    }
  }

  return (
    <div>
      <h2 className="page-title">酒桶追溯</h2>
      <p className="page-sub">按桶查看它在哪家酒吧、是否开桶、是否回收、是否清洗合格</p>
      <ErrorBox error={error} onClose={() => setError('')} />
      <div className="toolbar">
        <select value={filters.status} onChange={(e) => setFilters({ ...filters, status: e.target.value })}>
          <option value="">全部状态</option>
          {Object.entries(KEG_STATUS).map(([k, v]) => <option key={k} value={k}>{v}</option>)}
        </select>
        <select value={filters.batch_id} onChange={(e) => setFilters({ ...filters, batch_id: e.target.value })}>
          <option value="">全部批次</option>
          {meta.batches.map((b) => <option key={b.id} value={b.id}>{b.batch_no}（{b.style}）</option>)}
        </select>
        <input placeholder="搜索桶号…" value={filters.q} onChange={(e) => setFilters({ ...filters, q: e.target.value })} />
        <div className="spacer" />
        {['warehouse', 'admin'].includes(user.role) && (
          <button className="primary" onClick={() => setShowCreate(true)}>+ 登记酒桶</button>
        )}
      </div>

      <div className="card">
        {list.length === 0 ? <Empty /> : (
          <table>
            <thead>
              <tr><th>桶号</th><th>容量</th><th>批次</th><th>酒款</th><th>状态</th><th>所在酒吧</th><th>押金</th><th>可再灌装</th><th>到店日期</th></tr>
            </thead>
            <tbody>
              {list.map((k) => (
                <tr key={k.id} className="clickable" onClick={() => navigate(`/kegs/${k.id}`)}>
                  <td className="mono">{k.keg_no}</td>
                  <td>{k.capacity_liters}L</td>
                  <td className="mono">{k.batch_no}</td>
                  <td>{k.style}</td>
                  <td><Tag label={KEG_STATUS[k.status] || k.status} /></td>
                  <td>{k.bar_name || '—'}</td>
                  <td>¥{k.deposit_amount}</td>
                  <td>{k.refill_eligible ? '是' : <span style={{ color: 'var(--red)' }}>否</span>}</td>
                  <td className="muted">{k.delivered_at || '—'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {showCreate && (
        <Modal title="登记酒桶（灌装入库）" onClose={() => setShowCreate(false)}>
          <Field label="桶号">
            <input value={form.keg_no} onChange={(e) => setForm({ ...form, keg_no: e.target.value })} placeholder="如 KEG-1009" />
          </Field>
          <div className="field-row">
            <Field label="容量（L）">
              <select value={form.capacity_liters} onChange={(e) => setForm({ ...form, capacity_liters: e.target.value })}>
                <option value={30}>30L</option>
                <option value={50}>50L</option>
              </select>
            </Field>
            <Field label="押金（¥）">
              <input type="number" value={form.deposit_amount} onChange={(e) => setForm({ ...form, deposit_amount: e.target.value })} />
            </Field>
          </div>
          <Field label="灌装批次">
            <select value={form.batch_id} onChange={(e) => setForm({ ...form, batch_id: e.target.value })}>
              <option value="">请选择</option>
              {meta.batches.filter((b) => b.status === 'active').map((b) => (
                <option key={b.id} value={b.id}>{b.batch_no}（{b.style}）</option>
              ))}
            </select>
          </Field>
          <div className="actions">
            <button onClick={() => setShowCreate(false)}>取消</button>
            <button className="primary" onClick={create} disabled={!form.keg_no || !form.batch_id}>登记</button>
          </div>
        </Modal>
      )}
    </div>
  )
}
