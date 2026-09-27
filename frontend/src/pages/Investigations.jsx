import { useCallback, useEffect, useState } from 'react'
import { api } from '../api'
import { Empty, ErrorBox, Field, Modal, OkBox, Tag } from '../components'
import { BATCH_STATUS, INV_CONCLUSION, INV_STATUS, RESTOCK_STATUS } from '../labels'

function EvidenceTable({ list, tempLimit }) {
  return (
    <table>
      <thead>
        <tr>
          <th>酒吧 / 城市</th><th>桶号</th><th>灌装日期</th><th>送达（运输温度）</th><th>开桶日期</th>
          <th>出酒设备</th><th>口味现象</th><th>出酒/气压</th><th>客诉照片</th><th>详情 / 反馈时间</th>
        </tr>
      </thead>
      <tbody>
        {list.map((f) => (
          <tr key={f.id}>
            <td>{f.bar_name}<div className="muted">{f.city}</div></td>
            <td className="mono">{f.keg_no}</td>
            <td>{f.fill_date}</td>
            <td>
              {f.delivered_at || '—'}
              {f.temp_c ? (
                <div className={f.over_temp ? 'over-temp' : 'muted'}>
                  🌡️ {f.temp_c}℃{f.over_temp ? `（超 ${tempLimit}℃）` : ''}
                </div>
              ) : <div className="muted">温度未记录</div>}
            </td>
            <td>{f.opened_at || '未开桶'}</td>
            <td>{f.equipment || '—'}</td>
            <td>
              {f.taste_desc || '—'}
              {f.taste_issue && <div className="muted">口味异常</div>}
            </td>
            <td>
              {f.pour_speed}
              {f.pressure_issue && <div className="muted">⚠️ 气压</div>}
            </td>
            <td>
              {f.photos.length === 0 ? <span className="muted">—</span> : (
                <div className="evidence-photos">
                  {f.photos.map((p) => (
                    <a key={p} href={`/uploads/${p}`} target="_blank" rel="noreferrer">
                      <img src={`/uploads/${p}`} alt="客诉照片" />
                    </a>
                  ))}
                </div>
              )}
            </td>
            <td>
              {f.detail || '—'}
              <div className="muted">{f.created_at}</div>
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  )
}

function RestockPanel({ inv, user, onChanged }) {
  const [error, setError] = useState('')
  const [editId, setEditId] = useState(null)
  const [batchId, setBatchId] = useState('')
  const [count, setCount] = useState('')

  const canAssign = ['sales', 'warehouse', 'qc', 'admin'].includes(user.role)
  const canAdvance = ['warehouse', 'admin'].includes(user.role)

  const submitAssign = async (id) => {
    setError('')
    try {
      await api(`/restock/${id}/assign`, {
        method: 'POST',
        body: { replace_batch_id: batchId ? Number(batchId) : 0, keg_count: count ? Number(count) : 0 },
      })
      setEditId(null)
      onChanged()
    } catch (e) {
      setError(e.message)
    }
  }
  const advance = async (id, status) => {
    setError('')
    try {
      await api(`/restock/${id}/advance`, { method: 'POST', body: { status } })
      onChanged()
    } catch (e) {
      setError(e.message)
    }
  }

  return (
    <div className="card">
      <h3>换货备货任务（召回决定同步销售 + 仓库备货，{inv.restock_tasks.length}）</h3>
      <ErrorBox error={error} onClose={() => setError('')} />
      {inv.restock_tasks.length === 0 ? <Empty text="暂无备货任务（确认召回后按受影响酒吧自动生成）" /> : (
        <table>
          <thead>
            <tr><th>受影响酒吧</th><th>换货数量</th><th>可替换批次</th><th>状态</th><th>备注</th><th>操作</th></tr>
          </thead>
          <tbody>
            {inv.restock_tasks.map((t) => (
              <tr key={t.id}>
                <td>{t.bar_name}<div className="muted">{t.city}</div></td>
                <td>{t.keg_count} 桶</td>
                <td>
                  {t.replace_batch_no ? (
                    <span className="mono">{t.replace_batch_no}（{t.replace_style}）</span>
                  ) : <span className="muted">待销售确认</span>}
                </td>
                <td><Tag label={RESTOCK_STATUS[t.status] || t.status} /></td>
                <td className="muted">{t.note || '—'}</td>
                <td style={{ whiteSpace: 'nowrap' }}>
                  {canAssign && t.status !== 'done' && (
                    <button className="small" onClick={() => { setEditId(t.id); setBatchId(t.replace_batch_id || ''); setCount(t.keg_count || '') }}>
                      {t.replace_batch_id ? '调整批次' : '确认替换批次'}
                    </button>
                  )}{' '}
                  {canAdvance && t.status === 'open' && (
                    <button className="small primary" disabled={!t.replace_batch_id} onClick={() => advance(t.id, 'stocking')}>开始备货</button>
                  )}
                  {canAdvance && t.status === 'stocking' && (
                    <button className="small primary" onClick={() => advance(t.id, 'done')}>备货完成</button>
                  )}
                  {editId === t.id && (
                    <Modal title={`确认换货 · ${t.bar_name}`} onClose={() => setEditId(null)}>
                      <Field label="可替换批次（同酒款在售、仓库有现货）">
                        <select value={batchId} onChange={(e) => setBatchId(e.target.value)}>
                          <option value="">请选择</option>
                          {inv.replacements.map((r) => (
                            <option key={r.id} value={r.id}>
                              {r.batch_no} · 灌装 {r.fill_date} · 在库 {r.warehouse_stock} 桶
                            </option>
                          ))}
                        </select>
                      </Field>
                      <Field label="换货桶数">
                        <input type="number" min="1" value={count} onChange={(e) => setCount(e.target.value)} />
                      </Field>
                      <div className="actions">
                        <button onClick={() => setEditId(null)}>取消</button>
                        <button className="primary" disabled={!batchId || !count} onClick={() => submitAssign(t.id)}>确认</button>
                      </div>
                    </Modal>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  )
}

function ConcludePanel({ inv, onConcluded }) {
  const [conclusion, setConclusion] = useState('recall')
  const [note, setNote] = useState('')
  const [rate, setRate] = useState('30')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  const submit = async () => {
    setError('')
    setBusy(true)
    try {
      await api(`/investigations/${inv.id}/conclude`, {
        method: 'POST',
        body: { conclusion, note, discount_rate: conclusion === 'discount' ? Number(rate) : 0 },
      })
      onConcluded()
    } catch (e) {
      setError(e.message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="card" style={{ background: 'var(--panel2)' }}>
      <h3>品控结论（决定已开桶酒召回或折价，并解除/维持暂停配送）</h3>
      <ErrorBox error={error} onClose={() => setError('')} />
      <div className="field">
        <label style={{ display: 'flex', gap: 18, flexWrap: 'wrap' }}>
          {[
            ['recall', '确认召回（已开桶/未开桶停售回收，生成换货备货）'],
            ['discount', '已开桶折价（批次排除批量问题，恢复配送）'],
            ['clear', '排除批次问题（解除暂停，恢复配送）'],
          ].map(([v, label]) => (
            <span key={v}>
              <input type="radio" name="concl" style={{ width: 'auto', marginRight: 6 }} checked={conclusion === v} onChange={() => setConclusion(v)} />
              {label}
            </span>
          ))}
        </label>
      </div>
      {conclusion === 'discount' && (
        <Field label="折价比例（%，按已开桶押金折算补偿）">
          <input type="number" min="1" max="100" value={rate} onChange={(e) => setRate(e.target.value)} />
        </Field>
      )}
      <Field label="结论说明（同步酒吧 / 销售 / 仓库 / 财务 / 经销商）">
        <textarea value={note} onChange={(e) => setNote(e.target.value)}
          placeholder="如：实验室检测 TBA 超标，确认灌装环节氧化，全部召回；或：指标正常，判定为单店出酒设备管路老化，已开桶折价 30%" />
      </Field>
      <button className="danger" disabled={!note || busy} onClick={submit}>{busy ? '提交中…' : '出具品控结论'}</button>
    </div>
  )
}

function InvestigationDetail({ id, user, onClose, onChanged }) {
  const [inv, setInv] = useState(null)
  const [error, setError] = useState('')
  const load = useCallback(() => {
    api(`/investigations/${id}`).then(setInv).catch((e) => setError(e.message))
  }, [id])
  useEffect(load, [load])

  if (!inv) return <Modal title="口味异常批次排查" onClose={onClose} wide><Empty text="加载中…" /></Modal>

  const isQC = ['qc', 'admin'].includes(user.role)
  const overTempCount = inv.evidence.filter((e) => e.over_temp).length

  return (
    <Modal title={`口味异常排查 ${inv.investigation_no} · ${inv.batch_no}`} onClose={onClose} wide>
      <ErrorBox error={error} onClose={() => setError('')} />
      <div className="kv" style={{ marginBottom: 12 }}>
        <div><div className="k">批次 / 酒款</div><div className="v mono">{inv.batch_no}（{inv.style}）</div></div>
        <div><div className="k">排查状态</div><div className="v"><Tag label={INV_STATUS[inv.status] || inv.status} tone={inv.status === 'investigating' ? 'red' : 'green'} /></div></div>
        <div><div className="k">批次配送状态</div><div className="v"><Tag label={BATCH_STATUS[invStatusBatch(inv)] || invStatusBatch(inv)} tone={invStatusBatch(inv) === 'recalled' ? 'red' : invStatusBatch(inv) === 'active' ? 'green' : 'red'} /></div></div>
        <div><div className="k">投诉酒吧</div><div className="v">{inv.affected_bars.filter((b) => b.complained).length} 家</div></div>
        <div><div className="k">超温证据</div><div className="v">{overTempCount > 0 ? <span className="over-temp">{overTempCount} 条运输超温（&gt;{inv.temp_limit}℃）</span> : '—'}</div></div>
        <div><div className="k">建单时间</div><div className="v">{inv.created_at}</div></div>
      </div>
      {inv.status === 'investigating' && (
        <div className="error-box">
          排查中：该批次剩余酒桶已暂停配送（在库禁出库、待配送/配送中订单已暂缓、酒吧禁止开桶），待品控结论。
        </div>
      )}
      {inv.status === 'concluded' && (
        <div className="ok-box">
          品控结论：{INV_CONCLUSION[inv.conclusion] || inv.conclusion}
          {inv.conclusion === 'discount' ? ` · 折价 ${inv.discount_rate}%` : ''} —— {inv.conclusion_note}
          {inv.concluded_at ? `（${inv.concluded_at}）` : ''}
        </div>
      )}

      <div className="card">
        <h3>关联证据链（灌装时间 · 运输温度 · 开桶日期 · 出酒设备 · 客诉照片，{inv.evidence.length}）</h3>
        {inv.evidence.length === 0 ? <Empty /> : <EvidenceTable list={inv.evidence} tempLimit={inv.temp_limit} />}
      </div>

      <div className="card">
        <h3>受影响酒吧（销售可据此对接换货，{inv.affected_bars.length}）</h3>
        <table>
          <thead><tr><th>酒吧</th><th>城市 / 经销商</th><th>在店桶数</th><th>已开桶</th><th>是否已投诉</th></tr></thead>
          <tbody>
            {inv.affected_bars.map((b) => (
              <tr key={b.id}>
                <td>{b.name}</td>
                <td>{b.city}<div className="muted">{b.dealer_name}</div></td>
                <td>{b.keg_count}</td>
                <td>{b.opened_count}</td>
                <td>{b.complained ? <span className="over-temp">⚠️ 已投诉</span> : <span className="muted">持有库存</span>}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <div className="card">
        <h3>同酒款可替换批次（在售 · 仓库有现货，{inv.replacements.length}）</h3>
        {inv.replacements.length === 0 ? <Empty text="暂无同酒款在库批次，请先灌装新批次" /> : (
          <table>
            <thead><tr><th>批次号</th><th>灌装日期</th><th>最佳饮用期</th><th>仓库现货</th></tr></thead>
            <tbody>
              {inv.replacements.map((r) => (
                <tr key={r.id}>
                  <td className="mono">{r.batch_no}</td>
                  <td>{r.fill_date}</td>
                  <td>{r.best_before}</td>
                  <td>{r.warehouse_stock} 桶</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      <RestockPanel inv={inv} user={user} onChanged={load} />

      {isQC && inv.status === 'investigating' && <ConcludePanel inv={inv} onConcluded={() => { load(); onChanged() }} />}
    </Modal>
  )
}

function invStatusBatch(inv) {
  if (inv.status === 'investigating') return 'investigating'
  return inv.conclusion === 'recall' ? 'recalled' : 'active'
}

export default function Investigations({ user }) {
  const [list, setList] = useState([])
  const [error, setError] = useState('')
  const [detailId, setDetailId] = useState(null)
  const [filter, setFilter] = useState('')

  const load = useCallback(() => {
    api('/investigations').then(setList).catch((e) => setError(e.message))
  }, [])
  useEffect(load, [load])

  const shown = list.filter((x) => !filter || x.status === filter)

  return (
    <div>
      <h2 className="page-title">口味异常批次排查</h2>
      <p className="page-sub">多家酒吧反馈同批次氧化味时，平台关联灌装时间、运输温度、开桶日期、出酒设备与客诉照片；品控确认前暂停配送，结论决定召回或折价</p>
      <ErrorBox error={error} onClose={() => setError('')} />
      <div className="toolbar">
        <select value={filter} onChange={(e) => setFilter(e.target.value)}>
          <option value="">全部状态</option>
          {Object.entries(INV_STATUS).map(([k, v]) => <option key={k} value={k}>{v}</option>)}
        </select>
        <span className="muted">销售可见受影响酒吧与可替换批次；召回后在此确认换货批次</span>
      </div>
      <div className="card">
        {shown.length === 0 ? <Empty text="暂无口味异常排查单" /> : (
          <table>
            <thead>
              <tr><th>排查单号</th><th>批次</th><th>酒款</th><th>状态</th><th>投诉酒吧</th><th>品控结论</th><th>备货任务</th><th>建单时间</th><th></th></tr>
            </thead>
            <tbody>
              {shown.map((x) => (
                <tr key={x.id}>
                  <td className="mono">{x.investigation_no}</td>
                  <td className="mono">{x.batch_no}</td>
                  <td>{x.style}</td>
                  <td><Tag label={INV_STATUS[x.status] || x.status} /></td>
                  <td>{x.complaint_bars} 家</td>
                  <td>{x.status === 'concluded' ? INV_CONCLUSION[x.conclusion] || x.conclusion : <span className="muted">待品控结论</span>}</td>
                  <td>{x.restock_total > 0 ? `${x.restock_total} 个（待办 ${x.restock_open}）` : '—'}</td>
                  <td className="muted">{x.created_at}</td>
                  <td><button className="small" onClick={() => setDetailId(x.id)}>查看 / 处理</button></td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
      {detailId && (
        <InvestigationDetail id={detailId} user={user} onClose={() => setDetailId(null)} onChanged={load} />
      )}
    </div>
  )
}
