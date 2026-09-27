import { useCallback, useEffect, useState } from 'react'
import { api } from '../api'
import { Empty, ErrorBox, Field, Modal, OkBox, Tag } from '../components'
import { BATCH_STATUS, CASE_STATUS, CASE_TYPE, KEG_STATUS } from '../labels'

function BatchDetail({ id, user, onClose, onChanged }) {
  const [b, setB] = useState(null)
  const [error, setError] = useState('')
  const [ok, setOk] = useState('')
  const [reason, setReason] = useState('')
  const [recallResult, setRecallResult] = useState(null)

  const load = useCallback(() => {
    api(`/batches/${id}`).then(setB).catch((e) => setError(e.message))
  }, [id])
  useEffect(load, [load])

  const act = async (path) => {
    setError('')
    setOk('')
    try {
      const res = await api(path, { method: 'POST', body: { reason } })
      if (path.endsWith('/recall')) setRecallResult(res)
      setOk(path.endsWith('/recall') ? '已发起召回并通知相关方' : '已暂停销售并通知相关方')
      load()
      onChanged()
    } catch (e) {
      setError(e.message)
    }
  }

  if (!b) return <Modal title="批次详情" onClose={onClose} wide><Empty text="加载中…" /></Modal>

  const canOperate = ['qc', 'warehouse', 'admin'].includes(user.role)
  return (
    <Modal title={`批次 ${b.batch_no}`} onClose={onClose} wide>
      <ErrorBox error={error} onClose={() => setError('')} />
      <OkBox text={ok} onClose={() => setOk('')} />
      <div className="kv" style={{ marginBottom: 14 }}>
        <div><div className="k">酒款</div><div className="v">{b.style}</div></div>
        <div><div className="k">灌装日期</div><div className="v">{b.fill_date}</div></div>
        <div><div className="k">最佳饮用期</div><div className="v">{b.best_before}</div></div>
        <div><div className="k">状态</div><div className="v"><Tag label={BATCH_STATUS[b.status] || b.status} /></div></div>
        <div><div className="k">配送状态</div><div className="v">{b.delivery_hold ? <Tag label="暂停配送（排查中）" tone="amber" /> : '正常'}</div></div>
        {b.suspend_reason && <div><div className="k">停售/召回原因</div><div className="v">{b.suspend_reason}</div></div>}
      </div>

      {canOperate && b.status !== 'recalled' && (
        <div className="card" style={{ background: 'var(--panel2)' }}>
          <div className="field">
            <label>停售 / 召回原因</label>
            <input value={reason} onChange={(e) => setReason(e.target.value)} placeholder="例如：理化指标异常，疑似染菌" />
          </div>
          <div style={{ display: 'flex', gap: 10 }}>
            {b.status === 'active' && (
              <button className="danger" onClick={() => act(`/batches/${b.id}/suspend`)}>
                暂停销售（通知酒吧/待配送单/经销商）
              </button>
            )}
            {user.role !== 'warehouse' && (
              <button className="danger" onClick={() => act(`/batches/${b.id}/recall`)}>
                发起召回（区分已售/已开桶/未开桶）
              </button>
            )}
          </div>
        </div>
      )}

      {recallResult && (
        <div className="card">
          <h3>召回库存拆分</h3>
          <div className="chips" style={{ marginBottom: 10 }}>
            <span className="tag">已售卖 {recallResult.sold.length} 桶</span>
            <span className="tag">已开桶 {recallResult.opened.length} 桶</span>
            <span className="tag">未开桶 {recallResult.unopened.length} 桶</span>
            <span className="tag">已暂缓配送单 {recallResult.held_shipments} 个</span>
            <span className="tag">已通知酒吧 {recallResult.notified_bars} 家 / 经销商 {recallResult.notified_dealers} 家</span>
          </div>
          {[['已售卖（已打空/已回收）', recallResult.sold], ['已开桶（立即停售）', recallResult.opened], ['未开桶库存', recallResult.unopened]].map(([title, list]) => (
            <div key={title} style={{ marginBottom: 10 }}>
              <div className="muted" style={{ marginBottom: 4 }}>{title}</div>
              {list.length === 0 ? <div className="muted">—</div> : (
                <div className="chips">
                  {list.map((k) => (
                    <span key={k.id} className="tag mono">
                      {k.keg_no} · {KEG_STATUS[k.status] || k.status}{k.bar_name ? ` · ${k.bar_name}` : ''}
                    </span>
                  ))}
                </div>
              )}
            </div>
          ))}
        </div>
      )}

      <div className="card">
        <h3>批次酒桶（{b.kegs.length}）</h3>
        {b.kegs.length === 0 ? <Empty /> : (
          <table>
            <thead><tr><th>桶号</th><th>容量</th><th>状态</th><th>所在酒吧</th><th>押金</th><th>可再灌装</th></tr></thead>
            <tbody>
              {b.kegs.map((k) => (
                <tr key={k.id}>
                  <td className="mono">{k.keg_no}</td>
                  <td>{k.capacity_liters}L</td>
                  <td><Tag label={KEG_STATUS[k.status] || k.status} /></td>
                  <td>{k.bar_name || '—'}</td>
                  <td>¥{k.deposit_amount}</td>
                  <td>{k.refill_eligible ? '是' : '否'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      <div className="card">
        <h3>开桶反馈（{b.feedbacks.length}）</h3>
        {b.feedbacks.length === 0 ? <Empty /> : (
          <table>
            <thead><tr><th>桶号</th><th>酒吧</th><th>出酒</th><th>气压</th><th>口味异常</th><th>客诉</th><th>详情</th><th>时间</th></tr></thead>
            <tbody>
              {b.feedbacks.map((f) => (
                <tr key={f.id}>
                  <td className="mono">{f.keg_no}</td>
                  <td>{f.bar_name}</td>
                  <td>{f.pour_speed}</td>
                  <td>{f.pressure_issue ? '⚠️' : '—'}</td>
                  <td>{f.taste_issue ? '⚠️' : '—'}</td>
                  <td>{f.complaint ? '⚠️' : '—'}</td>
                  <td>{f.detail}</td>
                  <td className="muted">{f.created_at}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {b.cases.length > 0 && (
        <div className="card">
          <h3>关联工单</h3>
          <table>
            <thead><tr><th>工单号</th><th>类型</th><th>状态</th><th>标题</th></tr></thead>
            <tbody>
              {b.cases.map((c) => (
                <tr key={c.id}>
                  <td className="mono">{c.case_no}</td>
                  <td>{CASE_TYPE[c.case_type] || c.case_type}</td>
                  <td><Tag label={CASE_STATUS[c.status] || c.status} /></td>
                  <td>{c.title}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </Modal>
  )
}

export default function Batches({ user }) {
  const [list, setList] = useState([])
  const [error, setError] = useState('')
  const [detailId, setDetailId] = useState(null)
  const [showCreate, setShowCreate] = useState(false)
  const [form, setForm] = useState({ batch_no: '', style: 'IPA', fill_date: '', best_before: '' })

  const load = useCallback(() => {
    api('/batches').then(setList).catch((e) => setError(e.message))
  }, [])
  useEffect(load, [load])

  const create = async () => {
    setError('')
    try {
      await api('/batches', { method: 'POST', body: form })
      setShowCreate(false)
      setForm({ batch_no: '', style: 'IPA', fill_date: '', best_before: '' })
      load()
    } catch (e) {
      setError(e.message)
    }
  }

  return (
    <div>
      <h2 className="page-title">批次管理</h2>
      <p className="page-sub">IPA / 世涛 / 小麦 / 酸啤各批次的灌装、投诉聚合与停售召回</p>
      <ErrorBox error={error} onClose={() => setError('')} />
      <div className="toolbar">
        <div className="spacer" />
        {['warehouse', 'admin'].includes(user.role) && (
          <button className="primary" onClick={() => setShowCreate(true)}>+ 新建批次</button>
        )}
      </div>
      <div className="card">
        {list.length === 0 ? <Empty /> : (
          <table>
            <thead>
              <tr><th>批次号</th><th>酒款</th><th>灌装日期</th><th>最佳饮用期</th><th>状态</th><th>桶数</th><th>投诉(酒吧数)</th><th></th></tr>
            </thead>
            <tbody>
              {list.map((b) => (
                <tr key={b.id}>
                  <td className="mono">{b.batch_no}</td>
                  <td>{b.style}</td>
                  <td>{b.fill_date}</td>
                  <td>{b.best_before}</td>
                  <td>
                    <Tag label={BATCH_STATUS[b.status] || b.status} />
                    {b.delivery_hold && <> <Tag label="暂停配送" tone="amber" /></>}
                  </td>
                  <td>{b.keg_total}</td>
                  <td>{b.complaints > 0 ? <span style={{ color: 'var(--red)' }}>{b.complaints}（{b.complaint_bars}家）</span> : '—'}</td>
                  <td><button className="small" onClick={() => setDetailId(b.id)}>详情</button></td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {showCreate && (
        <Modal title="新建批次" onClose={() => setShowCreate(false)}>
          <Field label="批次号">
            <input value={form.batch_no} onChange={(e) => setForm({ ...form, batch_no: e.target.value })} placeholder="如 IPA-20260924" />
          </Field>
          <Field label="酒款">
            <select value={form.style} onChange={(e) => setForm({ ...form, style: e.target.value })}>
              {['IPA', '世涛', '小麦', '酸啤'].map((s) => <option key={s}>{s}</option>)}
            </select>
          </Field>
          <div className="field-row">
            <Field label="灌装日期">
              <input type="date" value={form.fill_date} onChange={(e) => setForm({ ...form, fill_date: e.target.value })} />
            </Field>
            <Field label="最佳饮用期">
              <input type="date" value={form.best_before} onChange={(e) => setForm({ ...form, best_before: e.target.value })} />
            </Field>
          </div>
          <div className="actions">
            <button onClick={() => setShowCreate(false)}>取消</button>
            <button className="primary" onClick={create} disabled={!form.batch_no || !form.fill_date || !form.best_before}>创建</button>
          </div>
        </Modal>
      )}

      {detailId && <BatchDetail id={detailId} user={user} onClose={() => setDetailId(null)} onChanged={load} />}
    </div>
  )
}
