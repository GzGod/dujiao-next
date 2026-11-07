// 新版本部署后，旧标签页仍引用已被替换的分片文件名，动态 import 会失败，表现为
// "点了没反应"或 "Failed to fetch dynamically imported module"。
// 检测到这类错误时整页刷新一次，拿到新版 index.html；用 sessionStorage 限制频率，避免死循环。

const RELOAD_KEY = 'dj_stale_chunk_reload_at'
const MIN_INTERVAL_MS = 10_000

const isChunkLoadError = (err: unknown) => {
  const message = String((err as any)?.message || err || '')
  return /Failed to fetch dynamically imported module|Importing a module script failed|error loading dynamically imported module|Unable to preload CSS/i.test(message)
}

/** 若是分片加载失败则刷新页面（可选跳到目标路径），返回是否已触发刷新。 */
export const reloadOnStaleChunk = (err: unknown, targetPath?: string) => {
  if (!isChunkLoadError(err)) return false
  try {
    const last = Number(sessionStorage.getItem(RELOAD_KEY) || 0)
    if (Date.now() - last < MIN_INTERVAL_MS) return false
    sessionStorage.setItem(RELOAD_KEY, String(Date.now()))
  } catch {
    // sessionStorage 不可用时仍刷新一次
  }
  if (targetPath) {
    window.location.assign(targetPath)
  } else {
    window.location.reload()
  }
  return true
}

/** 监听 Vite 的预加载失败事件。 */
export const installStaleChunkReload = () => {
  window.addEventListener('vite:preloadError', (event) => {
    if (reloadOnStaleChunk((event as any).payload)) {
      event.preventDefault()
    }
  })
}
