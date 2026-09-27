import { useEffect, useState } from 'react'
import { api } from '../api'
import { Empty, ErrorBox, Tag } from '../components'
import { KEG_STATUS } from '../labels'

export default function Dashboard({ user }) {
  const [data, setData] = useState(null)
  const [error, setError] = useState('')

  useEffect(() => {
    api('/dashboard').then(setData).catch((e) => setError(e.message))
  }, [])

  return (
    <div>
      <h2 className="page-title">仪表盘</h2>
      <p className="page-sub">酒桶流转、工单与押金的全局概览</p>
      <ErrorBox error={error} onClose={() => setError('')} />
      {!data ? (
        <Empty text="加载中…" />
      ) : (
        <>
          <div className="stat-grid">
            <div className="stat">
              <div className="num">{data.total_kegs}</div>
              <div className="lbl">酒桶总数</div>
            </div>
            <div className="stat">
              <div className="num">{data.at_bar_kegs}</div>
              <div className="lbl">在酒吧（未开+已开）</div>
            </div>
            <div className="stat">
              <div className="num">{data.active_batches}</div>
              <div className="lbl">在售批次</div>
            </div>
            <div className="stat">
              <div className="num" style={{ color: data.investigating_batches > 0 ? 'var(--red)' : undefined }}>{data.investigating_batches}</div>
              <div className="lbl">排查暂停批次</div>
            </div>
            <div className="stat">
              <div className="num">{data.open_cases}</div>
              <div className="lbl">未结工单</div>
            </div>
            <div className="stat">
              <div className="num">{data.pending_deposits}</div>
              <div className="lbl">待结算押金</div>
            </div>
            <div className="stat">
              <div className="num">{data.unread}</div>
              <div className="lbl">未读通知</div>
            </div>
          </div>

          <div className="card">
            <h3>酒桶状态分布</h3>
            <div className="chips">
              {data.keg_status.length === 0 && <Empty />}
              {data.keg_status.map((s) => (
                <span key={s.status} className="tag">
                  <Tag label={KEG_STATUS[s.status] || s.status} /> × {s.count}
                </span>
              ))}
            </div>
          </div>

          <div className="card">
            <h3>最新动态</h3>
            {data.recent_events.length === 0 ? (
              <Empty />
            ) : (
              <ul className="timeline">
                {data.recent_events.map((e) => (
                  <li key={e.id}>
                    <div>
                      <b className="mono">{e.keg_no}</b> · {e.event}
                      <span className="muted"> · {e.actor}</span>
                    </div>
                    <div className="muted">{e.detail}</div>
                    <div className="t">{e.time}</div>
                  </li>
                ))}
              </ul>
            )}
          </div>
        </>
      )}
    </div>
  )
}
