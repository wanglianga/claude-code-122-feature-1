import { useCallback, useEffect, useState } from 'react'
import { api } from '../api'
import { Empty, ErrorBox, Field, Modal, OkBox, Tag } from '../components'
import { CASE_STATUS, CASE_TYPE, ROLE } from '../labels'

function CaseDetail({ id, user, onClose, onChanged }) {
  const [c, setC] = useState(null)
  const [error, setError] = useState('')
  const [note, setNote] = useState('')
  const [resolution, setResolution] = useState('')
  const [deduction, setDeduction] = useState('')

  const load = useCallback(() => {
    api(`/cases/${id}`).then(setC).catch((e) => setError(e.message))
  }, [id])
  useEffect(load, [load])

  const addNote = async () => {
    setError('')
    try {
      await api(`/cases/${id}/notes`, { method: 'POST', body: { note } })
      setNote('')
      load()
      onChanged()
    } catch (e) {
      setError(e.message)
    }
  }

  const resolve = async () => {
    setError('')
    try {
      await api(`/cases/${id}/resolve`, {
        method: 'POST',
        body: { resolution, deduction_amount: deduction ? Number(deduction) : 0 },
      })
      setResolution('')
      setDeduction('')
      load()
      onChanged()
    } catch (e) {
      setError(e.message)
    }
  }

  if (!c) return <Modal title="工单详情" onClose={onClose} wide><Empty text="加载中…" /></Modal>
  const canResolve = ['warehouse', 'qc', 'finance', 'admin'].includes(user.role) && c.status !== 'resolved'

  return (
    <Modal title={`工单 ${c.case_no}`} onClose={onClose} wide>
      <ErrorBox error={error} onClose={() => setError('')} />
      <div className="kv" style={{ marginBottom: 14 }}>
        <div><div className="k">类型</div><div className="v">{CASE_TYPE[c.case_type] || c.case_type}</div></div>
        <div><div className="k">状态</div><div className="v"><Tag label={CASE_STATUS[c.status] || c.status} /></div></div>
        <div><div className="k">关联酒桶</div><div className="v mono">{c.keg_no || '—'}</div></div>
        <div><div className="k">关联批次</div><div className="v mono">{c.batch_no || '—'}</div></div>
        <div><div className="k">关联酒吧</div><div className="v">{c.bar_name || '—'}</div></div>
        <div><div className="k">创建时间</div><div className="v">{c.created_at}</div></div>
      </div>
      <div className="card" style={{ background: 'var(--panel2)' }}>
        <div style={{ fontWeight: 600, marginBottom: 4 }}>{c.title}</div>
        <div className="muted">{c.detail}</div>
        {c.resolution && (
          <div style={{ marginTop: 8, color: 'var(--green)' }}>处理结论：{c.resolution}</div>
        )}
      </div>

      <div className="card">
        <h3>协同记录（各角色在此同桶/同批处理）</h3>
        {c.notes.length === 0 ? <Empty text="暂无处理记录" /> : (
          c.notes.map((n, i) => (
            <div className="note-item" key={i}>
              <div>
                <b>{n.user_name}</b> <span className="tag">{ROLE[n.role] || n.role}</span>
                <span className="muted" style={{ fontSize: 12 }}>{n.time}</span>
              </div>
              <div>{n.note}</div>
            </div>
          ))
        )}
        <div className="field" style={{ marginTop: 12 }}>
          <label>补充处理意见</label>
          <textarea value={note} onChange={(e) => setNote(e.target.value)} placeholder="仓库/店长/司机/清洗厂/品控/财务均可留言" />
        </div>
        <button className="small" disabled={!note} onClick={addNote}>提交记录</button>
      </div>

      {canResolve && (
        <div className="card">
          <h3>结案处理</h3>
          <Field label="处理结论">
            <textarea value={resolution} onChange={(e) => setResolution(e.target.value)} placeholder="例如：酒桶已找回 / 按协议扣减 50% 押金 / 批次停售观察" />
          </Field>
          {c.keg_id && (
            <Field label="押金扣减金额（¥，可选）">
              <input type="number" value={deduction} onChange={(e) => setDeduction(e.target.value)} placeholder="0 表示不扣款" />
            </Field>
          )}
          <div className="actions">
            <button className="primary" disabled={!resolution} onClick={resolve}>结案</button>
          </div>
        </div>
      )}
    </Modal>
  )
}

export default function Cases({ user }) {
  const [list, setList] = useState([])
  const [filters, setFilters] = useState({ status: '', type: '' })
  const [error, setError] = useState('')
  const [detailId, setDetailId] = useState(null)

  const load = useCallback(() => {
    const qs = new URLSearchParams(Object.entries(filters).filter(([, v]) => v !== '')).toString()
    api('/cases' + (qs ? '?' + qs : '')).then(setList).catch((e) => setError(e.message))
  }, [filters])
  useEffect(load, [load])

  return (
    <div>
      <h2 className="page-title">协同工单</h2>
      <p className="page-sub">延迟归还 / 丢失 / 变形 / 批次投诉 / 漏扫 / 清洗不合格 —— 各角色在同一只桶或同一批酒中处理</p>
      <ErrorBox error={error} onClose={() => setError('')} />
      <div className="toolbar">
        <select value={filters.status} onChange={(e) => setFilters({ ...filters, status: e.target.value })}>
          <option value="">全部状态</option>
          {Object.entries(CASE_STATUS).map(([k, v]) => <option key={k} value={k}>{v}</option>)}
        </select>
        <select value={filters.type} onChange={(e) => setFilters({ ...filters, type: e.target.value })}>
          <option value="">全部类型</option>
          {Object.entries(CASE_TYPE).map(([k, v]) => <option key={k} value={k}>{v}</option>)}
        </select>
      </div>
      <div className="card">
        {list.length === 0 ? <Empty /> : (
          <table>
            <thead><tr><th>工单号</th><th>类型</th><th>状态</th><th>标题</th><th>酒桶</th><th>批次</th><th>酒吧</th><th>记录</th><th>创建时间</th><th></th></tr></thead>
            <tbody>
              {list.map((c) => (
                <tr key={c.id}>
                  <td className="mono">{c.case_no}</td>
                  <td>{CASE_TYPE[c.case_type] || c.case_type}</td>
                  <td><Tag label={CASE_STATUS[c.status] || c.status} /></td>
                  <td>{c.title}</td>
                  <td className="mono">{c.keg_no || '—'}</td>
                  <td className="mono">{c.batch_no || '—'}</td>
                  <td>{c.bar_name || '—'}</td>
                  <td>{c.note_count}</td>
                  <td className="muted">{c.created_at}</td>
                  <td><button className="small" onClick={() => setDetailId(c.id)}>处理</button></td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
      {detailId && <CaseDetail id={detailId} user={user} onClose={() => setDetailId(null)} onChanged={load} />}
    </div>
  )
}
