import { useCallback, useEffect, useState } from 'react'
import { useParams } from 'react-router-dom'
import { api } from '../api'
import { Empty, ErrorBox, Field, KV, Modal, OkBox, Tag } from '../components'
import { BATCH_STATUS, CASE_STATUS, CASE_TYPE, DEPOSIT_STATUS, ENTRY_TYPE, KEG_STATUS, POUR_SPEEDS } from '../labels'

export default function KegDetail({ user }) {
  const { id } = useParams()
  const [k, setK] = useState(null)
  const [meta, setMeta] = useState({ batches: [] })
  const [error, setError] = useState('')
  const [ok, setOk] = useState('')
  const [modal, setModal] = useState('') // feedback | wash | refill | deduct | lost
  const [fb, setFb] = useState({ pour_speed: '正常', pressure_issue: false, taste_issue: false, complaint: false, detail: '' })
  const [washNote, setWashNote] = useState('')
  const [refillBatch, setRefillBatch] = useState('')
  const [deduct, setDeduct] = useState({ amount: '', reason: '' })
  const [lostNote, setLostNote] = useState('')

  const load = useCallback(() => {
    api(`/kegs/${id}`).then(setK).catch((e) => setError(e.message))
  }, [id])
  useEffect(load, [load])
  useEffect(() => {
    api('/meta').then(setMeta).catch(() => {})
  }, [])

  const act = async (path, body) => {
    setError('')
    setOk('')
    try {
      await api(path, { method: 'POST', body })
      setModal('')
      setOk('操作成功')
      load()
    } catch (e) {
      setError(e.message)
    }
  }

  if (!k) return <div><ErrorBox error={error} onClose={() => setError('')} /><Empty text="加载中…" /></div>

  const isBar = user.role === 'bar_manager'
  const isWarehouse = ['warehouse', 'admin'].includes(user.role)
  const isWasher = ['washer', 'admin'].includes(user.role)
  const isFinance = ['finance', 'admin'].includes(user.role)

  return (
    <div>
      <h2 className="page-title">酒桶 {k.keg_no}</h2>
      <p className="page-sub">单桶全生命周期追溯</p>
      <ErrorBox error={error} onClose={() => setError('')} />
      <OkBox text={ok} onClose={() => setOk('')} />

      <div className="card">
        <KV items={[
          ['状态', <Tag key="s" label={KEG_STATUS[k.status] || k.status} />],
          ['批次', `${k.batch_no}（${k.style}）${k.batch_status !== 'active' ? ' · ' + (BATCH_STATUS[k.batch_status] || k.batch_status) : ''}`],
          ['容量', k.capacity_liters + 'L'],
          ['押金', '¥' + k.deposit_amount],
          ['所在酒吧', k.bar_name || '—'],
          ['可再灌装', k.refill_eligible ? '是' : '否'],
          ['到店时间', k.delivered_at || '—'],
          ['开桶时间', k.opened_at || '—'],
          ['打空时间', k.emptied_at || '—'],
          ['回收时间', k.returned_at || '—'],
        ]} />
        <div style={{ display: 'flex', gap: 10, marginTop: 16, flexWrap: 'wrap' }}>
          {isBar && k.status === 'at_bar' && (
            <button className="primary" onClick={() => act(`/kegs/${k.id}/open`)}>开桶</button>
          )}
          {isBar && (k.status === 'at_bar' || k.status === 'opened') && (
            <button onClick={() => setModal('feedback')}>开桶反馈</button>
          )}
          {isBar && k.status === 'opened' && (
            <button onClick={() => act(`/kegs/${k.id}/empty`)}>标记打空</button>
          )}
          {isBar && ['at_bar', 'opened', 'empty'].includes(k.status) && (
            <button className="danger" onClick={() => setModal('lost')}>登记丢失</button>
          )}
          {isWasher && (k.status === 'at_wash' || k.status === 'wash_failed') && (
            <button className="primary" onClick={() => setModal('wash')}>清洗检测登记</button>
          )}
          {isWarehouse && k.status === 'ready_refill' && k.refill_eligible && (
            <button className="primary" onClick={() => setModal('refill')}>再灌装入库</button>
          )}
          {isFinance && (
            <button className="danger" onClick={() => setModal('deduct')}>押金扣减</button>
          )}
        </div>
      </div>

      <div className="card">
        <h3>流转时间线</h3>
        {k.events.length === 0 ? <Empty /> : (
          <ul className="timeline">
            {k.events.map((e, i) => (
              <li key={i}>
                <div><b>{e.event}</b> <span className="muted">· {e.actor || '系统'}</span></div>
                <div className="muted">{e.detail}</div>
                <div className="t">{e.time}</div>
              </li>
            ))}
          </ul>
        )}
      </div>

      <div className="card">
        <h3>开桶反馈（{k.feedbacks.length}）</h3>
        {k.feedbacks.length === 0 ? <Empty /> : (
          <table>
            <thead><tr><th>酒吧</th><th>出酒</th><th>气压</th><th>口味</th><th>客诉</th><th>详情</th><th>时间</th></tr></thead>
            <tbody>
              {k.feedbacks.map((f, i) => (
                <tr key={i}>
                  <td>{f.bar_name}</td>
                  <td>{f.pour_speed}</td>
                  <td>{f.pressure_issue ? '⚠️' : '—'}</td>
                  <td>{f.taste_issue ? '⚠️' : '—'}</td>
                  <td>{f.complaint ? '⚠️' : '—'}</td>
                  <td>{f.detail}</td>
                  <td className="muted">{f.time}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      <div className="card">
        <h3>清洗检测记录（{k.wash_records.length}）</h3>
        {k.wash_records.length === 0 ? <Empty /> : (
          <table>
            <thead><tr><th>结果</th><th>说明</th><th>检测人</th><th>时间</th></tr></thead>
            <tbody>
              {k.wash_records.map((w, i) => (
                <tr key={i}>
                  <td><Tag label={w.result === 'pass' ? 'pass' : 'fail'} tone={w.result === 'pass' ? 'green' : 'red'} /> {w.result === 'pass' ? '合格' : '不合格'}</td>
                  <td>{w.note}</td>
                  <td>{w.washer}</td>
                  <td className="muted">{w.time}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      <div className="card">
        <h3>押金流水（{k.deposits.length}）</h3>
        {k.deposits.length === 0 ? <Empty /> : (
          <table>
            <thead><tr><th>类型</th><th>金额</th><th>账期</th><th>状态</th><th>事由</th><th>时间</th></tr></thead>
            <tbody>
              {k.deposits.map((d, i) => (
                <tr key={i}>
                  <td><Tag label={ENTRY_TYPE[d.entry_type] || d.entry_type} /></td>
                  <td>¥{d.amount}</td>
                  <td>{d.period}</td>
                  <td><Tag label={DEPOSIT_STATUS[d.status] || d.status} /></td>
                  <td>{d.reason}</td>
                  <td className="muted">{d.time}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {k.cases.length > 0 && (
        <div className="card">
          <h3>关联工单</h3>
          <table>
            <thead><tr><th>工单号</th><th>类型</th><th>状态</th><th>标题</th></tr></thead>
            <tbody>
              {k.cases.map((c) => (
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

      {modal === 'feedback' && (
        <Modal title={`开桶反馈 · ${k.keg_no}`} onClose={() => setModal('')}>
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
            <textarea value={fb.detail} onChange={(e) => setFb({ ...fb, detail: e.target.value })} placeholder="描述出酒、气压、口味或客诉情况" />
          </Field>
          <div className="actions">
            <button onClick={() => setModal('')}>取消</button>
            <button className="primary" onClick={() => act(`/kegs/${k.id}/feedback`, fb)}>提交反馈</button>
          </div>
        </Modal>
      )}

      {modal === 'wash' && (
        <Modal title={`清洗检测 · ${k.keg_no}`} onClose={() => setModal('')}>
          <Field label="检测说明">
            <textarea value={washNote} onChange={(e) => setWashNote(e.target.value)} placeholder="例如：内外壁清洗合格 / 内壁残留酒石" />
          </Field>
          <div className="actions">
            <button onClick={() => setModal('')}>取消</button>
            <button className="danger" onClick={() => act(`/kegs/${k.id}/wash`, { result: 'fail', note: washNote })}>不合格</button>
            <button className="primary" onClick={() => act(`/kegs/${k.id}/wash`, { result: 'pass', note: washNote })}>合格</button>
          </div>
        </Modal>
      )}

      {modal === 'refill' && (
        <Modal title={`再灌装 · ${k.keg_no}`} onClose={() => setModal('')}>
          <Field label="目标批次">
            <select value={refillBatch} onChange={(e) => setRefillBatch(e.target.value)}>
              <option value="">请选择</option>
              {meta.batches.filter((b) => b.status === 'active').map((b) => (
                <option key={b.id} value={b.id}>{b.batch_no}（{b.style}）</option>
              ))}
            </select>
          </Field>
          <div className="actions">
            <button onClick={() => setModal('')}>取消</button>
            <button className="primary" disabled={!refillBatch} onClick={() => act(`/kegs/${k.id}/refill`, { batch_id: Number(refillBatch) })}>确认再灌装</button>
          </div>
        </Modal>
      )}

      {modal === 'deduct' && (
        <Modal title={`押金扣减 · ${k.keg_no}`} onClose={() => setModal('')}>
          <Field label="扣减金额（¥）">
            <input type="number" value={deduct.amount} onChange={(e) => setDeduct({ ...deduct, amount: e.target.value })} />
          </Field>
          <Field label="扣减原因">
            <textarea value={deduct.reason} onChange={(e) => setDeduct({ ...deduct, reason: e.target.value })} placeholder="例如：逾期未归还 30 天，按协议扣减 50%" />
          </Field>
          <div className="actions">
            <button onClick={() => setModal('')}>取消</button>
            <button className="danger" disabled={!deduct.amount || !deduct.reason}
              onClick={() => act('/deposits/deduct', { keg_id: k.id, amount: Number(deduct.amount), reason: deduct.reason })}>
              确认扣减
            </button>
          </div>
        </Modal>
      )}

      {modal === 'lost' && (
        <Modal title={`登记丢失 · ${k.keg_no}`} onClose={() => setModal('')}>
          <Field label="情况说明">
            <textarea value={lostNote} onChange={(e) => setLostNote(e.target.value)} placeholder="例如：盘点时发现酒桶下落不明" />
          </Field>
          <div className="actions">
            <button onClick={() => setModal('')}>取消</button>
            <button className="danger" onClick={() => act(`/kegs/${k.id}/lost`, { note: lostNote })}>确认丢失</button>
          </div>
        </Modal>
      )}
    </div>
  )
}
