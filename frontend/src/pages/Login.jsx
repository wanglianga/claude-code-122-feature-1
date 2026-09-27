import { useState } from 'react'
import { api } from '../api'
import { ErrorBox } from '../components'

const DEMO = [
  ['warehouse', 'warehouse123', '酒厂仓库'],
  ['bar1', 'bar123', '酒吧店长·北京'],
  ['bar2', 'bar123', '酒吧店长·上海'],
  ['driver1', 'driver123', '配送司机'],
  ['washer', 'washer123', '清洗厂'],
  ['qc', 'qc123', '品控'],
  ['finance', 'finance123', '财务'],
  ['dealer1', 'dealer123', '经销商·北京'],
  ['sales1', 'sales123', '销售人员'],
  ['admin', 'admin123', '管理员'],
]

export default function Login({ onLogin }) {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  const doLogin = async (u, p) => {
    setError('')
    setLoading(true)
    try {
      const data = await api('/login', { method: 'POST', body: { username: u, password: p } })
      onLogin(data.token, data.user)
    } catch (e) {
      setError(e.message)
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="login-wrap">
      <div className="login-card">
        <h1>🍺 KegTrace</h1>
        <div className="sub">精酿酒吧空桶回收押金与批次追溯平台</div>
        <ErrorBox error={error} onClose={() => setError('')} />
        <form
          onSubmit={(e) => {
            e.preventDefault()
            doLogin(username, password)
          }}
        >
          <div className="field">
            <label>用户名</label>
            <input value={username} onChange={(e) => setUsername(e.target.value)} autoFocus />
          </div>
          <div className="field">
            <label>密码</label>
            <input type="password" value={password} onChange={(e) => setPassword(e.target.value)} />
          </div>
          <button className="primary" style={{ width: '100%' }} disabled={loading || !username || !password}>
            {loading ? '登录中…' : '登 录'}
          </button>
        </form>
        <div className="muted" style={{ margin: '16px 0 8px', fontSize: 12 }}>
          演示账号（点击直接登录）：
        </div>
        <div className="demo-accounts">
          {DEMO.map(([u, p, label]) => (
            <button key={u} type="button" onClick={() => doLogin(u, p)} disabled={loading}>
              {label}
              <div className="muted mono" style={{ fontSize: 11 }}>
                {u}
              </div>
            </button>
          ))}
        </div>
      </div>
    </div>
  )
}
