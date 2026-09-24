import { useCallback, useEffect, useState } from 'react'
import { api } from '../api'
import { Empty, ErrorBox, Field, Modal, OkBox } from '../components'
import { ROLE } from '../labels'

// 基础数据维护（仅管理员）：用户 / 经销商 / 酒吧。
export default function Admin({ user }) {
  const [users, setUsers] = useState([])
  const [dealers, setDealers] = useState([])
  const [bars, setBars] = useState([])
  const [error, setError] = useState('')
  const [ok, setOk] = useState('')
  const [modal, setModal] = useState('') // user | dealer | bar
  const [uform, setUform] = useState({ username: '', password: '', name: '', role: 'bar_manager', city: '', dealer_id: '', bar_id: '' })
  const [dform, setDform] = useState({ name: '', city: '', contact: '' })
  const [bform, setBform] = useState({ name: '', city: '', dealer_id: '', address: '', manager_name: '' })

  const load = useCallback(() => {
    api('/admin/users').then(setUsers).catch((e) => setError(e.message))
    api('/admin/dealers').then(setDealers).catch(() => {})
    api('/admin/bars').then(setBars).catch(() => {})
  }, [])
  useEffect(load, [load])

  const create = async (path, body, done) => {
    setError('')
    setOk('')
    try {
      await api(path, { method: 'POST', body })
      setOk('创建成功')
      setModal('')
      done && done()
      load()
    } catch (e) {
      setError(e.message)
    }
  }

  if (user.role !== 'admin') return <Empty text="仅管理员可访问" />

  return (
    <div>
      <h2 className="page-title">基础数据</h2>
      <p className="page-sub">多城市经销商、合作酒吧与各角色账号</p>
      <ErrorBox error={error} onClose={() => setError('')} />
      <OkBox text={ok} onClose={() => setOk('')} />

      <div className="card">
        <h3>经销商（{dealers.length}）<button className="small primary" style={{ float: 'right' }} onClick={() => setModal('dealer')}>+ 新增</button></h3>
        <table>
          <thead><tr><th>ID</th><th>名称</th><th>城市</th><th>联系方式</th></tr></thead>
          <tbody>
            {dealers.map((d) => <tr key={d.id}><td>{d.id}</td><td>{d.name}</td><td>{d.city}</td><td>{d.contact}</td></tr>)}
          </tbody>
        </table>
      </div>

      <div className="card">
        <h3>酒吧（{bars.length}）<button className="small primary" style={{ float: 'right' }} onClick={() => setModal('bar')}>+ 新增</button></h3>
        <table>
          <thead><tr><th>ID</th><th>名称</th><th>城市</th><th>所属经销商</th><th>地址</th><th>店长</th></tr></thead>
          <tbody>
            {bars.map((b) => <tr key={b.id}><td>{b.id}</td><td>{b.name}</td><td>{b.city}</td><td>{b.dealer_name}</td><td>{b.address}</td><td>{b.manager_name}</td></tr>)}
          </tbody>
        </table>
      </div>

      <div className="card">
        <h3>用户（{users.length}）<button className="small primary" style={{ float: 'right' }} onClick={() => setModal('user')}>+ 新增</button></h3>
        <table>
          <thead><tr><th>ID</th><th>用户名</th><th>姓名</th><th>角色</th><th>城市</th><th>经销商</th><th>酒吧</th></tr></thead>
          <tbody>
            {users.map((u) => (
              <tr key={u.id}>
                <td>{u.id}</td><td className="mono">{u.username}</td><td>{u.name}</td>
                <td>{ROLE[u.role] || u.role}</td><td>{u.city || '—'}</td>
                <td>{u.dealer_name || '—'}</td><td>{u.bar_name || '—'}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {modal === 'dealer' && (
        <Modal title="新增经销商" onClose={() => setModal('')}>
          <Field label="名称"><input value={dform.name} onChange={(e) => setDform({ ...dform, name: e.target.value })} /></Field>
          <Field label="城市"><input value={dform.city} onChange={(e) => setDform({ ...dform, city: e.target.value })} /></Field>
          <Field label="联系方式"><input value={dform.contact} onChange={(e) => setDform({ ...dform, contact: e.target.value })} /></Field>
          <div className="actions">
            <button onClick={() => setModal('')}>取消</button>
            <button className="primary" disabled={!dform.name || !dform.city}
              onClick={() => create('/admin/dealers', dform, () => setDform({ name: '', city: '', contact: '' }))}>创建</button>
          </div>
        </Modal>
      )}

      {modal === 'bar' && (
        <Modal title="新增酒吧" onClose={() => setModal('')}>
          <Field label="名称"><input value={bform.name} onChange={(e) => setBform({ ...bform, name: e.target.value })} /></Field>
          <div className="field-row">
            <Field label="城市"><input value={bform.city} onChange={(e) => setBform({ ...bform, city: e.target.value })} /></Field>
            <Field label="所属经销商">
              <select value={bform.dealer_id} onChange={(e) => setBform({ ...bform, dealer_id: e.target.value })}>
                <option value="">请选择</option>
                {dealers.map((d) => <option key={d.id} value={d.id}>{d.name}（{d.city}）</option>)}
              </select>
            </Field>
          </div>
          <Field label="地址"><input value={bform.address} onChange={(e) => setBform({ ...bform, address: e.target.value })} /></Field>
          <Field label="店长姓名"><input value={bform.manager_name} onChange={(e) => setBform({ ...bform, manager_name: e.target.value })} /></Field>
          <div className="actions">
            <button onClick={() => setModal('')}>取消</button>
            <button className="primary" disabled={!bform.name || !bform.city || !bform.dealer_id}
              onClick={() => create('/admin/bars', { ...bform, dealer_id: Number(bform.dealer_id) }, () => setBform({ name: '', city: '', dealer_id: '', address: '', manager_name: '' }))}>创建</button>
          </div>
        </Modal>
      )}

      {modal === 'user' && (
        <Modal title="新增用户" onClose={() => setModal('')}>
          <div className="field-row">
            <Field label="用户名"><input value={uform.username} onChange={(e) => setUform({ ...uform, username: e.target.value })} /></Field>
            <Field label="初始密码"><input value={uform.password} onChange={(e) => setUform({ ...uform, password: e.target.value })} /></Field>
          </div>
          <div className="field-row">
            <Field label="姓名"><input value={uform.name} onChange={(e) => setUform({ ...uform, name: e.target.value })} /></Field>
            <Field label="角色">
              <select value={uform.role} onChange={(e) => setUform({ ...uform, role: e.target.value })}>
                {Object.entries(ROLE).map(([k, v]) => <option key={k} value={k}>{v}</option>)}
              </select>
            </Field>
          </div>
          <div className="field-row">
            <Field label="城市"><input value={uform.city} onChange={(e) => setUform({ ...uform, city: e.target.value })} /></Field>
            <Field label="所属经销商（经销商角色）">
              <select value={uform.dealer_id} onChange={(e) => setUform({ ...uform, dealer_id: e.target.value })}>
                <option value="">无</option>
                {dealers.map((d) => <option key={d.id} value={d.id}>{d.name}</option>)}
              </select>
            </Field>
            <Field label="所属酒吧（店长角色）">
              <select value={uform.bar_id} onChange={(e) => setUform({ ...uform, bar_id: e.target.value })}>
                <option value="">无</option>
                {bars.map((b) => <option key={b.id} value={b.id}>{b.name}</option>)}
              </select>
            </Field>
          </div>
          <div className="actions">
            <button onClick={() => setModal('')}>取消</button>
            <button className="primary" disabled={!uform.username || !uform.password || !uform.name}
              onClick={() => create('/admin/users', {
                username: uform.username, password: uform.password, name: uform.name, role: uform.role, city: uform.city,
                dealer_id: uform.dealer_id ? Number(uform.dealer_id) : null,
                bar_id: uform.bar_id ? Number(uform.bar_id) : null,
              }, () => setUform({ username: '', password: '', name: '', role: 'bar_manager', city: '', dealer_id: '', bar_id: '' }))}>创建</button>
          </div>
        </Modal>
      )}
    </div>
  )
}
