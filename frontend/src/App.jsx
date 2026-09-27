import { useCallback, useEffect, useState } from 'react'
import { HashRouter, NavLink, Navigate, Route, Routes, useNavigate } from 'react-router-dom'
import { api, setToken } from './api'
import { ROLE } from './labels'
import Login from './pages/Login'
import Dashboard from './pages/Dashboard'
import Batches from './pages/Batches'
import Kegs from './pages/Kegs'
import KegDetail from './pages/KegDetail'
import Shipments from './pages/Shipments'
import BarOps from './pages/BarOps'
import Pickups from './pages/Pickups'
import Wash from './pages/Wash'
import Cases from './pages/Cases'
import Deposits from './pages/Deposits'
import Notifications from './pages/Notifications'
import Investigations from './pages/Investigations'
import Admin from './pages/Admin'

const MENU = [
  { path: '/', label: '仪表盘', roles: null },
  { path: '/batches', label: '批次管理', roles: ['admin', 'warehouse', 'qc', 'finance', 'dealer', 'sales'] },
  { path: '/investigations', label: '批次排查', roles: ['admin', 'qc', 'warehouse', 'sales', 'finance', 'dealer'] },
  { path: '/kegs', label: '酒桶追溯', roles: null },
  { path: '/shipments', label: '配送订单', roles: ['admin', 'warehouse', 'driver', 'bar_manager', 'dealer', 'sales'] },
  { path: '/bar', label: '酒吧操作', roles: ['bar_manager'] },
  { path: '/pickups', label: '回收预约', roles: ['admin', 'warehouse', 'driver', 'bar_manager'] },
  { path: '/wash', label: '清洗检测', roles: ['admin', 'washer', 'qc'] },
  { path: '/cases', label: '协同工单', roles: null },
  { path: '/deposits', label: '押金台账', roles: ['admin', 'finance', 'bar_manager', 'warehouse', 'dealer'] },
  { path: '/notifications', label: '通知中心', roles: null },
  { path: '/admin', label: '基础数据', roles: ['admin'] },
]

function Layout({ user, onLogout, children }) {
  const [unread, setUnread] = useState(0)
  const navigate = useNavigate()
  const loadUnread = useCallback(() => {
    api('/notifications')
      .then((list) => setUnread(list.filter((n) => !n.read).length))
      .catch(() => {})
  }, [])
  useEffect(() => {
    loadUnread()
    const t = setInterval(loadUnread, 20000)
    return () => clearInterval(t)
  }, [loadUnread])

  return (
    <div className="layout">
      <aside className="sidebar">
        <div className="logo">
          🍺 KegTrace
          <small>空桶押金 · 批次追溯</small>
        </div>
        {MENU.filter((m) => !m.roles || m.roles.includes(user.role)).map((m) => (
          <NavLink key={m.path} to={m.path} end={m.path === '/'} className={({ isActive }) => 'menu-item' + (isActive ? ' active' : '')}>
            {m.label}
          </NavLink>
        ))}
      </aside>
      <div className="main">
        <div className="topbar">
          <div className="who">
            <b>{user.name}</b> · {ROLE[user.role] || user.role}
            {user.city ? ` · ${user.city}` : ''}
          </div>
          <div style={{ display: 'flex', alignItems: 'center' }}>
            <span className="bell" style={{ cursor: 'pointer' }} onClick={() => navigate('/notifications')}>
              🔔
              {unread > 0 && <span className="dot">{unread}</span>}
            </span>
            <button className="small" onClick={onLogout}>
              退出登录
            </button>
          </div>
        </div>
        <div className="content">{children}</div>
      </div>
    </div>
  )
}

export default function App() {
  const [user, setUser] = useState(() => {
    try {
      return JSON.parse(localStorage.getItem('keg_user'))
    } catch {
      return null
    }
  })

  const handleLogin = (token, u) => {
    setToken(token)
    localStorage.setItem('keg_user', JSON.stringify(u))
    setUser(u)
  }
  const handleLogout = () => {
    setToken('')
    localStorage.removeItem('keg_user')
    setUser(null)
  }

  if (!user) return <Login onLogin={handleLogin} />

  return (
    <HashRouter>
      <Layout user={user} onLogout={handleLogout}>
        <Routes>
          <Route path="/" element={<Dashboard user={user} />} />
          <Route path="/batches" element={<Batches user={user} />} />
          <Route path="/investigations" element={<Investigations user={user} />} />
          <Route path="/kegs" element={<Kegs user={user} />} />
          <Route path="/kegs/:id" element={<KegDetail user={user} />} />
          <Route path="/shipments" element={<Shipments user={user} />} />
          <Route path="/bar" element={<BarOps user={user} />} />
          <Route path="/pickups" element={<Pickups user={user} />} />
          <Route path="/wash" element={<Wash user={user} />} />
          <Route path="/cases" element={<Cases user={user} />} />
          <Route path="/deposits" element={<Deposits user={user} />} />
          <Route path="/notifications" element={<Notifications />} />
          <Route path="/admin" element={<Admin user={user} />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </Layout>
    </HashRouter>
  )
}
