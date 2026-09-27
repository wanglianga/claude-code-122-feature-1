let token = localStorage.getItem('keg_token') || ''

export function setToken(t) {
  token = t || ''
  if (t) localStorage.setItem('keg_token', t)
  else localStorage.removeItem('keg_token')
}

export async function api(path, { method = 'GET', body } = {}) {
  const res = await fetch('/api' + path, {
    method,
    headers: {
      'Content-Type': 'application/json',
      ...(token ? { Authorization: 'Bearer ' + token } : {}),
    },
    body: body ? JSON.stringify(body) : undefined,
  })
  if (res.status === 401) {
    setToken('')
    localStorage.removeItem('keg_user')
    window.location.reload()
    throw new Error('登录已过期')
  }
  const data = await res.json().catch(() => ({}))
  if (!res.ok) throw new Error(data.error || `请求失败 (${res.status})`)
  return data
}

// uploadFile 上传客诉照片，返回 { name, url }。
export async function uploadFile(file) {
  const fd = new FormData()
  fd.append('file', file)
  const res = await fetch('/api/uploads', {
    method: 'POST',
    headers: token ? { Authorization: 'Bearer ' + token } : {},
    body: fd,
  })
  if (res.status === 401) {
    setToken('')
    localStorage.removeItem('keg_user')
    window.location.reload()
    throw new Error('登录已过期')
  }
  const data = await res.json().catch(() => ({}))
  if (!res.ok) throw new Error(data.error || `上传失败 (${res.status})`)
  return data
}
