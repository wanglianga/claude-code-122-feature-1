import { useCallback, useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api } from '../api'
import { Empty, ErrorBox, Field, Modal, OkBox, Tag } from '../components'
import { KEG_STATUS } from '../labels'

// 清洗厂：待清洗 / 曾不合格酒桶的检测登记。
export default function Wash({ user }) {
  const [kegs, setKegs] = useState([])
  const [error, setError] = useState('')
  const [ok, setOk] = useState('')
  const [target, setTarget] = useState(null)
  const [note, setNote] = useState('')
  const navigate = useNavigate()

  const load = useCallback(() => {
    Promise.all([api('/kegs?status=at_wash'), api('/kegs?status=wash_failed')])
      .then(([a, b]) => setKegs([...a, ...b]))
      .catch((e) => setError(e.message))
  }, [])
  useEffect(load, [load])

  const wash = async (result) => {
    setError('')
    setOk('')
    try {
      await api(`/kegs/${target.id}/wash`, { method: 'POST', body: { result, note } })
      setOk(result === 'pass' ? '已登记合格，酒桶可再灌装' : '已登记不合格，已生成工单')
      setTarget(null)
      setNote('')
      load()
    } catch (e) {
      setError(e.message)
    }
  }

  const canWash = ['washer', 'admin'].includes(user.role)

  return (
    <div>
      <h2 className="page-title">清洗检测</h2>
      <p className="page-sub">回收空桶的清洗与检测，合格后方可再灌装</p>
      <ErrorBox error={error} onClose={() => setError('')} />
      <OkBox text={ok} onClose={() => setOk('')} />
      <div className="card">
        {kegs.length === 0 ? <Empty text="暂无待清洗酒桶" /> : (
          <table>
            <thead><tr><th>桶号</th><th>批次</th><th>容量</th><th>状态</th><th>来自酒吧</th><th>回收时间</th><th></th></tr></thead>
            <tbody>
              {kegs.map((k) => (
                <tr key={k.id}>
                  <td className="mono" style={{ cursor: 'pointer' }} onClick={() => navigate(`/kegs/${k.id}`)}>{k.keg_no}</td>
                  <td className="mono">{k.batch_no}</td>
                  <td>{k.capacity_liters}L</td>
                  <td><Tag label={KEG_STATUS[k.status] || k.status} /></td>
                  <td>{k.bar_name || '—'}</td>
                  <td className="muted">{k.delivered_at || '—'}</td>
                  <td>
                    {canWash && (
                      <button className="small primary" onClick={() => { setTarget(k); setNote('') }}>登记检测</button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {target && (
        <Modal title={`清洗检测 · ${target.keg_no}`} onClose={() => setTarget(null)}>
          <Field label="检测说明">
            <textarea value={note} onChange={(e) => setNote(e.target.value)} placeholder="例如：内外壁清洗合格，残液已清空 / 内壁残留酒石需返工" />
          </Field>
          <div className="actions">
            <button onClick={() => setTarget(null)}>取消</button>
            <button className="danger" onClick={() => wash('fail')}>不合格</button>
            <button className="primary" onClick={() => wash('pass')}>合格</button>
          </div>
        </Modal>
      )}
    </div>
  )
}
