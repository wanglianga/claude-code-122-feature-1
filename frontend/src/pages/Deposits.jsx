import { useCallback, useEffect, useMemo, useState } from 'react'
import { api } from '../api'
import { Empty, ErrorBox, OkBox, Tag } from '../components'
import { DEPOSIT_STATUS, ENTRY_TYPE } from '../labels'

// 押金台账：按账期汇总 + 逐笔流水；财务可结算 / 触发逾期扫描。
export default function Deposits({ user }) {
  const [list, setList] = useState([])
  const [summary, setSummary] = useState([])
  const [filters, setFilters] = useState({ period: '', status: '' })
  const [error, setError] = useState('')
  const [ok, setOk] = useState('')

  const load = useCallback(() => {
    const qs = new URLSearchParams(Object.entries(filters).filter(([, v]) => v !== '')).toString()
    api('/deposits' + (qs ? '?' + qs : '')).then(setList).catch((e) => setError(e.message))
    api('/deposits/summary').then(setSummary).catch(() => {})
  }, [filters])
  useEffect(load, [load])

  const isFinance = ['finance', 'admin'].includes(user.role)

  const settle = async (id) => {
    setError('')
    setOk('')
    try {
      await api(`/deposits/${id}/settle`, { method: 'POST' })
      setOk('已结算')
      load()
    } catch (e) {
      setError(e.message)
    }
  }

  const scanLate = async () => {
    setError('')
    setOk('')
    try {
      const res = await api('/checks/late-returns', { method: 'POST' })
      setOk(`扫描完成，新生成 ${res.created} 个逾期工单`)
      load()
    } catch (e) {
      setError(e.message)
    }
  }

  // 汇总成 账期 → {charge, release, deduction} 的卡片
  const periods = useMemo(() => {
    const map = {}
    for (const s of summary) {
      map[s.period] = map[s.period] || {}
      map[s.period][s.entry_type + '_' + s.status] = s.total
    }
    return Object.entries(map).sort((a, b) => (a[0] < b[0] ? 1 : -1))
  }, [summary])

  const fmt = (v) => '¥' + (v || 0).toFixed(2)

  return (
    <div>
      <h2 className="page-title">押金台账</h2>
      <p className="page-sub">按桶收取 / 释放 / 扣减押金，按账期（月）汇总</p>
      <ErrorBox error={error} onClose={() => setError('')} />
      <OkBox text={ok} onClose={() => setOk('')} />

      <div className="stat-grid">
        {periods.map(([period, v]) => (
          <div className="stat" key={period}>
            <div className="lbl">账期 {period}</div>
            <div style={{ marginTop: 6, lineHeight: 1.9 }}>
              <div>收取：{fmt(v.charge_settled)}</div>
              <div style={{ color: 'var(--green)' }}>已释放：{fmt(v.release_settled)}</div>
              <div style={{ color: 'var(--red)' }}>已扣减：{fmt(v.deduction_settled)}</div>
              {(v.release_pending || v.deduction_pending) && (
                <div className="muted">
                  待结算：释放 {fmt(v.release_pending)} / 扣减 {fmt(v.deduction_pending)}
                </div>
              )}
            </div>
          </div>
        ))}
      </div>

      <div className="toolbar">
        <select value={filters.period} onChange={(e) => setFilters({ ...filters, period: e.target.value })}>
          <option value="">全部账期</option>
          {periods.map(([p]) => <option key={p} value={p}>{p}</option>)}
        </select>
        <select value={filters.status} onChange={(e) => setFilters({ ...filters, status: e.target.value })}>
          <option value="">全部状态</option>
          {Object.entries(DEPOSIT_STATUS).map(([k, v]) => <option key={k} value={k}>{v}</option>)}
        </select>
        <div className="spacer" />
        {['warehouse', 'finance', 'admin'].includes(user.role) && (
          <button onClick={scanLate}>扫描逾期未归还</button>
        )}
      </div>

      <div className="card">
        {list.length === 0 ? <Empty /> : (
          <table>
            <thead><tr><th>ID</th><th>桶号</th><th>酒吧</th><th>类型</th><th>金额</th><th>账期</th><th>状态</th><th>事由</th><th>时间</th><th></th></tr></thead>
            <tbody>
              {list.map((d) => (
                <tr key={d.id}>
                  <td className="muted">{d.id}</td>
                  <td className="mono">{d.keg_no}</td>
                  <td>{d.bar_name}</td>
                  <td><Tag label={ENTRY_TYPE[d.entry_type] || d.entry_type} /></td>
                  <td>¥{d.amount}</td>
                  <td>{d.period}</td>
                  <td><Tag label={DEPOSIT_STATUS[d.status] || d.status} /></td>
                  <td>{d.reason}</td>
                  <td className="muted">{d.created_at}</td>
                  <td>
                    {isFinance && d.status === 'pending' && (
                      <button className="small primary" onClick={() => settle(d.id)}>结算</button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  )
}
