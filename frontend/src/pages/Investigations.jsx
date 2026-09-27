import { useCallback, useEffect, useState } from 'react'
import { api } from '../api'
import { Empty, ErrorBox, Field, Modal, OkBox, PhotoThumbs, Tag } from '../components'
import { BATCH_STATUS, CONCLUSION, INVEST_STATUS, KEG_STATUS, SHIPMENT_STATUS } from '../labels'

function InvestDetail({ id, user, onClose, onChanged }) {
  const [d, setD] = useState(null)
  const [error, setError] = useState('')
  const [ok, setOk] = useState('')
  const [conclusion, setConclusion] = useState('recall')
  const [note, setNote] = useState('')
  const [discount, setDiscount] = useState('30')

  const load = useCallback(() => {
    api(`/investigations/${id}`).then(setD).catch((e) => setError(e.message))
  }, [id])
  useEffect(load, [load])

  const conclude = async () => {
    setError('')
    setOk('')
    try {
      await api(`/investigations/${id}/conclude`, {
        method: 'POST',
        body: { conclusion, note, discount_percent: conclusion === 'discount' ? Number(discount) : undefined },
      })
      setOk('已提交品控结论并同步相关方')
      load()
      onChanged()
    } catch (e) {
      setError(e.message)
    }
  }

  if (!d) return <Modal title="排查详情" onClose={onClose} wide><Empty text="加载中…" /></Modal>
  const b = d.batch || {}
  const canConclude = ['qc', 'admin'].includes(user.role) && d.status === 'investigating'

  return (
    <Modal title={`口味异常排查 ${d.inv_no}`} onClose={onClose} wide>
      <ErrorBox error={error} onClose={() => setError('')} />
      <OkBox text={ok} onClose={() => setOk('')} />

      <div className="kv" style={{ marginBottom: 14 }}>
        <div><div className="k">批次</div><div className="v mono">{b.batch_no}（{b.style}）</div></div>
        <div><div className="k">灌装日期</div><div className="v">{b.fill_date}</div></div>
        <div><div className="k">最佳饮用期</div><div className="v">{b.best_before}</div></div>
        <div><div className="k">批次状态</div><div className="v"><Tag label={BATCH_STATUS[b.status] || b.status} /></div></div>
        <div><div className="k">配送状态</div><div className="v">{b.delivery_hold ? <Tag label="暂停配送" tone="amber" /> : '正常'}</div></div>
        <div><div className="k">排查状态</div><div className="v"><Tag label={INVEST_STATUS[d.status] || d.status} /></div></div>
        {d.conclusion && (
          <div><div className="k">品控结论</div><div className="v">
            <Tag label={CONCLUSION[d.conclusion] || d.conclusion} />
            {d.conclusion === 'discount' && d.discount_percent != null && ` · 折价 ${d.discount_percent}%`}
          </div></div>
        )}
        {d.concluded_at && <div><div className="k">结论时间</div><div className="v">{d.concluded_at} · {d.concluded_by}</div></div>}
      </div>
      <div className="card" style={{ background: 'var(--panel2)' }}>
        <div style={{ fontWeight: 600, marginBottom: 4 }}>触发说明</div>
        <div className="muted">{d.trigger_detail}</div>
        {d.conclusion_note && <div style={{ marginTop: 6, color: 'var(--green)' }}>结论说明：{d.conclusion_note}</div>}
      </div>

      <div className="card">
        <h3>客诉反馈（出酒设备 / 客诉照片）</h3>
        {d.feedbacks.length === 0 ? <Empty /> : (
          <table>
            <thead><tr><th>酒吧</th><th>桶号</th><th>出酒设备</th><th>口味异常</th><th>客诉</th><th>照片</th><th>详情</th><th>时间</th></tr></thead>
            <tbody>
              {d.feedbacks.map((f) => (
                <tr key={f.id}>
                  <td>{f.bar_name}</td>
                  <td className="mono">{f.keg_no}</td>
                  <td>{f.equipment || '—'}</td>
                  <td>{f.taste_issue ? '⚠️' : '—'}</td>
                  <td>{f.complaint ? '⚠️' : '—'}</td>
                  <td><PhotoThumbs photos={f.photos} /></td>
                  <td>{f.detail}</td>
                  <td className="muted">{f.created_at}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      <div className="card">
        <h3>运输温度（冷链记录）</h3>
        {d.shipments.length === 0 ? <Empty text="该批次暂无配送记录" /> : (
          <table>
            <thead><tr><th>配送单</th><th>收货酒吧</th><th>状态</th><th>运输温度</th><th>发货</th><th>送达</th><th>本批桶数</th></tr></thead>
            <tbody>
              {d.shipments.map((s) => (
                <tr key={s.id}>
                  <td className="mono">{s.order_no}</td>
                  <td>{s.bar_name}</td>
                  <td><Tag label={SHIPMENT_STATUS[s.status] || s.status} /></td>
                  <td>{s.transport_temp_c != null ? (
                    <span style={{ color: s.transport_temp_c > 8 ? 'var(--red)' : 'inherit', fontWeight: s.transport_temp_c > 8 ? 600 : 400 }}>
                      {s.transport_temp_c}℃{s.transport_temp_c > 8 ? ' ⚠️超温' : ''}
                    </span>
                  ) : '—'}</td>
                  <td className="muted">{s.started_at || '—'}</td>
                  <td className="muted">{s.delivered_at || '—'}</td>
                  <td>{s.batch_kegs}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      <div className="card">
        <h3>批次酒桶（酒吧开桶日期）</h3>
        {d.kegs.length === 0 ? <Empty /> : (
          <table>
            <thead><tr><th>桶号</th><th>状态</th><th>所在酒吧</th><th>到店日期</th><th>开桶日期</th></tr></thead>
            <tbody>
              {d.kegs.map((k) => (
                <tr key={k.id}>
                  <td className="mono">{k.keg_no}</td>
                  <td><Tag label={KEG_STATUS[k.status] || k.status} /></td>
                  <td>{k.bar_name || '—'}</td>
                  <td className="muted">{k.delivered_at || '—'}</td>
                  <td className="muted">{k.opened_at || '—'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      <div className="card">
        <h3>受影响酒吧（{d.affected_bars.length}）</h3>
        {d.affected_bars.length === 0 ? <Empty text="暂无酒吧持有该批次酒桶" /> : (
          <table>
            <thead><tr><th>酒吧</th><th>城市</th><th>经销商</th><th>在店(未开+已开)</th><th>其中已开桶</th><th>已打空/预约回收</th></tr></thead>
            <tbody>
              {d.affected_bars.map((a) => (
                <tr key={a.bar_id}>
                  <td>{a.bar_name}</td>
                  <td>{a.city}</td>
                  <td>{a.dealer_name || '—'}</td>
                  <td>{a.in_stock}</td>
                  <td>{a.opened}</td>
                  <td>{a.emptied}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      <div className="card">
        <h3>可替换批次（同酒款 · 在售 · 未暂停配送）</h3>
        {d.replacements.length === 0 ? <Empty text="暂无同酒款可替换批次" /> : (
          <table>
            <thead><tr><th>批次号</th><th>酒款</th><th>最佳饮用期</th><th>在库可发桶数</th></tr></thead>
            <tbody>
              {d.replacements.map((r) => (
                <tr key={r.id}>
                  <td className="mono">{r.batch_no}</td>
                  <td>{r.style}</td>
                  <td>{r.best_before}</td>
                  <td>{r.in_warehouse}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {canConclude && (
        <div className="card">
          <h3>品控结论（决定已开桶酒召回或折价；同步销售与仓库备货）</h3>
          <div className="field-row">
            <Field label="结论">
              <select value={conclusion} onChange={(e) => setConclusion(e.target.value)}>
                <option value="recall">召回（含已开桶，批次召回）</option>
                <option value="discount">折价（已开桶折价结算，批次停售）</option>
                <option value="release">排除异常（恢复配送）</option>
              </select>
            </Field>
            {conclusion === 'discount' && (
              <Field label="折价比例（%）">
                <input type="number" min="1" max="100" value={discount} onChange={(e) => setDiscount(e.target.value)} />
              </Field>
            )}
          </div>
          <Field label="结论说明">
            <textarea value={note} onChange={(e) => setNote(e.target.value)} placeholder="例如：理化检测确认氧化，召回已开桶；或运输超温导致，已开桶折价 30%" />
          </Field>
          <div className="actions">
            <button className="danger" onClick={conclude}>提交品控结论</button>
          </div>
        </div>
      )}
    </Modal>
  )
}

export default function Investigations({ user }) {
  const [list, setList] = useState([])
  const [error, setError] = useState('')
  const [ok, setOk] = useState('')
  const [detailId, setDetailId] = useState(null)
  const [showCreate, setShowCreate] = useState(false)
  const [batches, setBatches] = useState([])
  const [form, setForm] = useState({ batch_id: '', trigger_detail: '' })

  const load = useCallback(() => {
    api('/investigations').then(setList).catch((e) => setError(e.message))
  }, [])
  useEffect(load, [load])
  useEffect(() => {
    api('/meta').then((m) => setBatches(m.batches || [])).catch(() => {})
  }, [])

  const create = async () => {
    setError('')
    try {
      await api('/investigations', { method: 'POST', body: { batch_id: Number(form.batch_id), trigger_detail: form.trigger_detail } })
      setShowCreate(false)
      setForm({ batch_id: '', trigger_detail: '' })
      setOk('已建立排查单，该批次已暂停配送')
      load()
    } catch (e) {
      setError(e.message)
    }
  }

  return (
    <div>
      <h2 className="page-title">口味异常批次排查</h2>
      <p className="page-sub">多家酒吧反馈同批次口味异常时自动建立排查：关联灌装时间、运输温度、开桶日期、出酒设备与客诉照片；品控确认前批次暂停配送</p>
      <ErrorBox error={error} onClose={() => setError('')} />
      <OkBox text={ok} onClose={() => setOk('')} />
      <div className="toolbar">
        <div className="spacer" />
        {['qc', 'warehouse', 'admin'].includes(user.role) && (
          <button className="primary" onClick={() => setShowCreate(true)}>+ 手动建立排查</button>
        )}
      </div>
      <div className="card">
        {list.length === 0 ? <Empty /> : (
          <table>
            <thead>
              <tr><th>排查单号</th><th>批次</th><th>酒款</th><th>批次状态</th><th>排查状态</th><th>品控结论</th><th>投诉(酒吧数)</th><th>创建时间</th><th></th></tr>
            </thead>
            <tbody>
              {list.map((i) => (
                <tr key={i.id}>
                  <td className="mono">{i.inv_no}</td>
                  <td className="mono">{i.batch_no}</td>
                  <td>{i.style}</td>
                  <td><Tag label={BATCH_STATUS[i.batch_status] || i.batch_status} /></td>
                  <td><Tag label={INVEST_STATUS[i.status] || i.status} /></td>
                  <td>{i.conclusion ? <Tag label={CONCLUSION[i.conclusion] || i.conclusion} /> : '—'}</td>
                  <td>{i.complaints > 0 ? <span style={{ color: 'var(--red)' }}>{i.complaints}（{i.complaint_bars}家）</span> : '—'}</td>
                  <td className="muted">{i.created_at}</td>
                  <td><button className="small" onClick={() => setDetailId(i.id)}>详情</button></td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {showCreate && (
        <Modal title="手动建立口味排查" onClose={() => setShowCreate(false)}>
          <Field label="批次">
            <select value={form.batch_id} onChange={(e) => setForm({ ...form, batch_id: e.target.value })}>
              <option value="">请选择</option>
              {batches.filter((b) => b.status === 'active').map((b) => (
                <option key={b.id} value={b.id}>{b.batch_no}（{b.style}）</option>
              ))}
            </select>
          </Field>
          <Field label="排查说明">
            <textarea value={form.trigger_detail} onChange={(e) => setForm({ ...form, trigger_detail: e.target.value })} placeholder="例如：经销商集中反馈某批次 IPA 有氧化味" />
          </Field>
          <div className="actions">
            <button onClick={() => setShowCreate(false)}>取消</button>
            <button className="primary" disabled={!form.batch_id} onClick={create}>建立并暂停配送</button>
          </div>
        </Modal>
      )}

      {detailId && <InvestDetail id={detailId} user={user} onClose={() => setDetailId(null)} onChanged={load} />}
    </div>
  )
}
