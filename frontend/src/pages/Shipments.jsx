import { useCallback, useEffect, useState } from 'react'
import { api } from '../api'
import { Empty, ErrorBox, Field, Modal, OkBox, Tag } from '../components'
import { KEG_STATUS, SHIPMENT_STATUS } from '../labels'

function CreateShipment({ meta, onClose, onCreated }) {
  const [form, setForm] = useState({ bar_id: '', driver_id: '', is_festival: false, transport_temp_c: '', note: '' })
  const [kegs, setKegs] = useState([])
  const [selected, setSelected] = useState([])
  const [error, setError] = useState('')

  useEffect(() => {
    api('/kegs?status=in_warehouse').then(setKegs).catch((e) => setError(e.message))
  }, [])

  const toggle = (id) => setSelected((s) => (s.includes(id) ? s.filter((x) => x !== id) : [...s, id]))

  const create = async () => {
    setError('')
    try {
      await api('/shipments', {
        method: 'POST',
        body: {
          ...form,
          bar_id: Number(form.bar_id),
          driver_id: Number(form.driver_id),
          keg_ids: selected,
          transport_temp_c: form.transport_temp_c === '' ? undefined : Number(form.transport_temp_c),
        },
      })
      onCreated()
    } catch (e) {
      setError(e.message)
    }
  }

  return (
    <Modal title="新建配送单" onClose={onClose} wide>
      <ErrorBox error={error} onClose={() => setError('')} />
      <div className="field-row">
        <Field label="收货酒吧">
          <select value={form.bar_id} onChange={(e) => setForm({ ...form, bar_id: e.target.value })}>
            <option value="">请选择</option>
            {meta.bars.map((b) => <option key={b.id} value={b.id}>{b.name}（{b.city} · {b.dealer_name}）</option>)}
          </select>
        </Field>
        <Field label="配送司机">
          <select value={form.driver_id} onChange={(e) => setForm({ ...form, driver_id: e.target.value })}>
            <option value="">请选择</option>
            {meta.drivers.map((d) => <option key={d.id} value={d.id}>{d.name}</option>)}
          </select>
        </Field>
      </div>
      <div className="field">
        <label>
          <input type="checkbox" style={{ width: 'auto' }} checked={form.is_festival}
            onChange={(e) => setForm({ ...form, is_festival: e.target.checked })} /> 节日临时铺货
        </label>
      </div>
      <Field label="运输温度（℃，冷链记录，可选）">
        <input type="number" step="0.1" value={form.transport_temp_c} onChange={(e) => setForm({ ...form, transport_temp_c: e.target.value })} placeholder="如 4.0" />
      </Field>
      <Field label="备注">
        <input value={form.note} onChange={(e) => setForm({ ...form, note: e.target.value })} placeholder="如：中秋国庆临时铺货" />
      </Field>
      <Field label={`选择在库酒桶（已选 ${selected.length} 只）`}>
        <div style={{ maxHeight: 220, overflowY: 'auto', border: '1px solid var(--line)', borderRadius: 8, padding: 8 }}>
          {kegs.length === 0 && <Empty text="仓库暂无可用酒桶" />}
          {kegs.map((k) => (
            <label key={k.id} style={{ display: 'flex', gap: 8, padding: '4px 2px', cursor: 'pointer' }}>
              <input type="checkbox" style={{ width: 'auto' }} checked={selected.includes(k.id)} onChange={() => toggle(k.id)} />
              <span className="mono">{k.keg_no}</span>
              <span className="muted">{k.capacity_liters}L · {k.batch_no}（{k.style}）· 押金¥{k.deposit_amount}</span>
            </label>
          ))}
        </div>
      </Field>
      <div className="actions">
        <button onClick={onClose}>取消</button>
        <button className="primary" disabled={!form.bar_id || !form.driver_id || selected.length === 0} onClick={create}>
          创建配送单
        </button>
      </div>
    </Modal>
  )
}

function ShipmentDetail({ id, onClose }) {
  const [s, setS] = useState(null)
  const [error, setError] = useState('')
  useEffect(() => {
    api(`/shipments/${id}`).then(setS).catch((e) => setError(e.message))
  }, [id])
  if (!s) return <Modal title="配送单详情" onClose={onClose} wide><Empty text="加载中…" /></Modal>
  return (
    <Modal title={`配送单 ${s.order_no}`} onClose={onClose} wide>
      <ErrorBox error={error} onClose={() => setError('')} />
      <div className="kv" style={{ marginBottom: 14 }}>
        <div><div className="k">状态</div><div className="v"><Tag label={SHIPMENT_STATUS[s.status] || s.status} /></div></div>
        <div><div className="k">经销商</div><div className="v">{s.dealer_name}</div></div>
        <div><div className="k">收货酒吧</div><div className="v">{s.bar_name}</div></div>
        <div><div className="k">司机</div><div className="v">{s.driver_name}</div></div>
        <div><div className="k">节日临时铺货</div><div className="v">{s.is_festival ? '是' : '否'}</div></div>
        <div><div className="k">收货人</div><div className="v">{s.receiver_name || '—'}</div></div>
        <div><div className="k">运输温度</div><div className="v">{s.transport_temp_c != null ? `${s.transport_temp_c}℃` : '—'}</div></div>
        <div><div className="k">创建</div><div className="v">{s.created_at}</div></div>
        <div><div className="k">签收</div><div className="v">{s.received_at || '—'}</div></div>
        {s.note && <div><div className="k">备注</div><div className="v">{s.note}</div></div>}
      </div>
      <table>
        <thead><tr><th>桶号</th><th>容量</th><th>批次</th><th>状态</th><th>押金</th></tr></thead>
        <tbody>
          {s.kegs.map((k) => (
            <tr key={k.id}>
              <td className="mono">{k.keg_no}</td>
              <td>{k.capacity_liters}L</td>
              <td className="mono">{k.batch_no}</td>
              <td><Tag label={KEG_STATUS[k.status] || k.status} /></td>
              <td>¥{k.deposit_amount}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </Modal>
  )
}

export default function Shipments({ user }) {
  const [list, setList] = useState([])
  const [meta, setMeta] = useState({ bars: [], drivers: [] })
  const [error, setError] = useState('')
  const [ok, setOk] = useState('')
  const [showCreate, setShowCreate] = useState(false)
  const [detailId, setDetailId] = useState(null)
  const [receiveId, setReceiveId] = useState(null)
  const [receiver, setReceiver] = useState('')
  const [deliverId, setDeliverId] = useState(null)
  const [deliverTemp, setDeliverTemp] = useState('')

  const load = useCallback(() => {
    api('/shipments').then(setList).catch((e) => setError(e.message))
  }, [])
  useEffect(load, [load])
  useEffect(() => {
    api('/meta').then(setMeta).catch(() => {})
  }, [])

  const act = async (path, body) => {
    setError('')
    setOk('')
    try {
      await api(path, { method: 'POST', body })
      setOk('操作成功')
      setReceiveId(null)
      setReceiver('')
      setDeliverId(null)
      setDeliverTemp('')
      load()
    } catch (e) {
      setError(e.message)
    }
  }

  return (
    <div>
      <h2 className="page-title">配送订单</h2>
      <p className="page-sub">酒厂 → 经销商 → 酒吧的酒桶配送，含节日临时铺货</p>
      <ErrorBox error={error} onClose={() => setError('')} />
      <OkBox text={ok} onClose={() => setOk('')} />
      <div className="toolbar">
        <div className="spacer" />
        {['warehouse', 'admin'].includes(user.role) && (
          <button className="primary" onClick={() => setShowCreate(true)}>+ 新建配送单</button>
        )}
      </div>
      <div className="card">
        {list.length === 0 ? <Empty /> : (
          <table>
            <thead>
              <tr><th>单号</th><th>酒吧</th><th>经销商</th><th>司机</th><th>桶数</th><th>状态</th><th>临时铺货</th><th>创建时间</th><th>操作</th></tr>
            </thead>
            <tbody>
              {list.map((s) => (
                <tr key={s.id}>
                  <td className="mono">{s.order_no}</td>
                  <td>{s.bar_name}</td>
                  <td>{s.dealer_name}</td>
                  <td>{s.driver_name}</td>
                  <td>{s.keg_count}</td>
                  <td><Tag label={SHIPMENT_STATUS[s.status] || s.status} /></td>
                  <td>{s.is_festival ? '🎉' : '—'}</td>
                  <td className="muted">{s.created_at}</td>
                  <td style={{ whiteSpace: 'nowrap' }}>
                    <button className="small" onClick={() => setDetailId(s.id)}>详情</button>{' '}
                    {user.role === 'driver' && s.status === 'pending' && (
                      <button className="small primary" onClick={() => act(`/shipments/${s.id}/start`)}>开始配送</button>
                    )}
                    {user.role === 'driver' && s.status === 'in_transit' && (
                      <button className="small primary" onClick={() => setDeliverId(s.id)}>送达</button>
                    )}
                    {user.role === 'bar_manager' && (s.status === 'delivered' || s.status === 'in_transit') && (
                      <button className="small primary" onClick={() => setReceiveId(s.id)}>签收</button>
                    )}
                    {['warehouse', 'admin'].includes(user.role) && (s.status === 'pending' || s.status === 'held') && (
                      <button className="small danger" onClick={() => act(`/shipments/${s.id}/cancel`)}>取消</button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {showCreate && <CreateShipment meta={meta} onClose={() => setShowCreate(false)} onCreated={() => { setShowCreate(false); load() }} />}
      {detailId && <ShipmentDetail id={detailId} onClose={() => setDetailId(null)} />}
      {receiveId && (
        <Modal title="确认签收" onClose={() => setReceiveId(null)}>
          <Field label="收货人姓名">
            <input value={receiver} onChange={(e) => setReceiver(e.target.value)} placeholder="酒吧收货人" />
          </Field>
          <div className="actions">
            <button onClick={() => setReceiveId(null)}>取消</button>
            <button className="primary" disabled={!receiver} onClick={() => act(`/shipments/${receiveId}/receive`, { receiver_name: receiver })}>
              确认签收（收取押金）
            </button>
          </div>
        </Modal>
      )}
      {deliverId && (
        <Modal title="确认送达" onClose={() => setDeliverId(null)}>
          <Field label="实际运输温度（℃，可选）">
            <input type="number" step="0.1" value={deliverTemp} onChange={(e) => setDeliverTemp(e.target.value)} placeholder="冷链车实测温度，如 4.0" />
          </Field>
          <div className="actions">
            <button onClick={() => setDeliverId(null)}>取消</button>
            <button className="primary" onClick={() => act(`/shipments/${deliverId}/deliver`, deliverTemp === '' ? {} : { transport_temp_c: Number(deliverTemp) })}>
              确认送达
            </button>
          </div>
        </Modal>
      )}
    </div>
  )
}
