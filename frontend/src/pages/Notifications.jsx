import { useCallback, useEffect, useState } from 'react'
import { api } from '../api'
import { Empty, ErrorBox, Tag } from '../components'

const NTYPE = {
  case: '工单',
  deposit: '押金',
  shipment: '配送',
  pickup: '回收',
  wash: '清洗',
  batch_suspend: '批次停售',
  batch_recall: '批次召回',
  investigation: '批次排查',
  restock: '换货备货',
}

export default function Notifications() {
  const [list, setList] = useState([])
  const [error, setError] = useState('')

  const load = useCallback(() => {
    api('/notifications').then(setList).catch((e) => setError(e.message))
  }, [])
  useEffect(load, [load])

  const readAll = async () => {
    try {
      await api('/notifications/read', { method: 'POST' })
      load()
    } catch (e) {
      setError(e.message)
    }
  }

  return (
    <div>
      <h2 className="page-title">通知中心</h2>
      <p className="page-sub">工单、押金、配送、批次停售/召回等消息</p>
      <ErrorBox error={error} onClose={() => setError('')} />
      <div className="toolbar">
        <div className="spacer" />
        <button onClick={readAll}>全部标记已读</button>
      </div>
      <div className="card">
        {list.length === 0 ? <Empty text="暂无通知" /> : (
          list.map((n) => (
            <div className="note-item" key={n.id} style={{ opacity: n.read ? 0.55 : 1 }}>
              <div>
                <Tag label={NTYPE[n.ntype] || n.ntype} tone={n.ntype === 'batch_recall' || n.ntype === 'investigation' ? 'red' : n.ntype === 'batch_suspend' || n.ntype === 'restock' ? 'amber' : 'blue'} />{' '}
                <b>{n.title}</b>
                {!n.read && <span style={{ color: 'var(--red)' }}> ●</span>}
                <span className="muted" style={{ float: 'right', fontSize: 12 }}>{n.created_at}</span>
              </div>
              <div className="muted">{n.content}</div>
            </div>
          ))
        )}
      </div>
    </div>
  )
}
