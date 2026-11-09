<script setup>
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Button from 'primevue/button'
import { HubService } from '../../../bindings/changeme'
import { useHubStore } from '../../stores/hub'
import { useServerStore } from '../../stores/servers'
import { useSettingStore } from '../../stores/SettingConf'
import { useLanguageStore } from '../../stores/Language'

const props = defineProps({
  id: {
    type: String,
    default: undefined
  },
  deviceId: {
    type: String,
    default: undefined
  }
})

const route = useRoute()
const router = useRouter()
const hub = useHubStore()
const serverStore = useServerStore()
const settingStore = useSettingStore()
const lang = useLanguageStore()

const canvasRef = ref(null)
const stageRef = ref(null)
const toolbarRef = ref(null)
const status = ref('connecting')
const errorText = ref('')
const fps = ref(0)
const frameSize = ref({ width: 0, height: 0 })
const confirmVisible = ref(false)
const requesting = ref(false)

const supported = typeof window !== 'undefined' && 'VideoDecoder' in window
const server = computed(() => serverStore.getById(props.id))
const device = computed(() => hub.devices.find((item) => item.deviceId === props.deviceId))
const deviceName = computed(() => device.value?.name || props.deviceId || '')

/* ---------- 悬浮工具栏 ---------- */

const TOOLBAR_POS_KEY = 'openremote.toolbarPos'
const TOOLBAR_PIN_KEY = 'openremote.toolbarPinned'
const QUALITY_KEY = 'openremote.quality'
const FPS_KEY = 'openremote.fps'
const HIDE_DELAY = 3000

function loadToolbarPos() {
  try {
    const saved = JSON.parse(localStorage.getItem(TOOLBAR_POS_KEY) || 'null')
    if (saved && Number.isFinite(saved.x) && Number.isFinite(saved.y)) return saved
  } catch {
    // 忽略损坏的本地数据
  }
  return null
}

const toolbarPos = ref(loadToolbarPos())
const toolbarVisible = ref(true)
const pinned = ref(localStorage.getItem(TOOLBAR_PIN_KEY) === '1')
const qualityOpen = ref(false)
const qualityUp = ref(false)
const quality = ref(localStorage.getItem(QUALITY_KEY) || 'original')
const fpsSetting = ref(Number(localStorage.getItem(FPS_KEY)) || 30)
const fpsOptions = [15, 30, 60]

// 工具栏位置：默认悬浮在右上角，拖动后改为绝对定位。
const toolbarStyle = computed(() =>
  toolbarPos.value
    ? { left: `${toolbarPos.value.x}px`, top: `${toolbarPos.value.y}px`, right: 'auto', transform: 'none' }
    : {}
)
const revealStyle = computed(() =>
  toolbarPos.value
    ? { left: `${toolbarPos.value.x + 12}px`, top: `${toolbarPos.value.y + 6}px`, right: 'auto', transform: 'none' }
    : {}
)

const qualityOptions = computed(() => [
  {
    key: 'original',
    label: lang.t('remote.qualityOriginal'),
    hint: lang.t('remote.qualityOriginalHint'),
    maxWidth: 0,
    bitrate: 4000
  },
  {
    key: 'hd',
    label: lang.t('remote.qualityHD'),
    hint: '1920 px',
    maxWidth: 1920,
    bitrate: 2500
  },
  {
    key: 'smooth',
    label: lang.t('remote.qualitySmooth'),
    hint: '1280 px',
    maxWidth: 1280,
    bitrate: 1200
  },
  {
    key: 'low',
    label: lang.t('remote.qualityLow'),
    hint: '960 px',
    maxWidth: 960,
    bitrate: 700
  }
])
const currentQuality = computed(
  () => qualityOptions.value.find((item) => item.key === quality.value) ?? qualityOptions.value[0]
)

/* ---------- 文件传输与剪切板 ---------- */

const FILE_CHUNK = 256 * 1024
const TYPE_FILE = 2
const TYPE_VIDEO = 1

const filePanelOpen = ref(false)
const fileInputRef = ref(null)
const uploads = ref([])
const remoteFiles = ref([])
const downloading = ref('')
const clipboardHint = ref('')
let fileSeq = 1
let hintTimer = 0
const downloads = new Map()

let hideTimer = 0
let hoveringToolbar = false
let dragState = null

function clearHideTimer() {
  if (hideTimer) {
    window.clearTimeout(hideTimer)
    hideTimer = 0
  }
}

// 无操作几秒后自动淡出工具栏，固定显示或文件面板打开时不再隐藏。
function scheduleHide() {
  clearHideTimer()
  if (pinned.value || filePanelOpen.value) return
  hideTimer = window.setTimeout(() => {
    if (!hoveringToolbar && !qualityOpen.value && !filePanelOpen.value) toolbarVisible.value = false
  }, HIDE_DELAY)
}

function revealToolbar() {
  toolbarVisible.value = true
  scheduleHide()
}

function onToolbarEnter() {
  hoveringToolbar = true
  toolbarVisible.value = true
  clearHideTimer()
}

function onToolbarLeave() {
  hoveringToolbar = false
  scheduleHide()
}

function togglePin() {
  pinned.value = !pinned.value
  localStorage.setItem(TOOLBAR_PIN_KEY, pinned.value ? '1' : '0')
  if (pinned.value) {
    toolbarVisible.value = true
    clearHideTimer()
    return
  }
  scheduleHide()
}

function onToolbarPointerDown(event) {
  if (event.button !== 0) return
  if (event.target.closest('button, .quality-menu')) return
  const stage = stageRef.value
  const bar = toolbarRef.value
  if (!stage || !bar) return

  const stageRect = stage.getBoundingClientRect()
  const barRect = bar.getBoundingClientRect()
  dragState = {
    dx: event.clientX - barRect.left,
    dy: event.clientY - barRect.top,
    stageRect,
    width: barRect.width,
    height: barRect.height
  }
  try {
    bar.setPointerCapture(event.pointerId)
  } catch {
    // 合成事件没有活动指针时忽略
  }
  window.addEventListener('pointermove', onToolbarPointerMove)
  window.addEventListener('pointerup', onToolbarPointerUp)
  event.preventDefault()
}

function onToolbarPointerMove(event) {
  if (!dragState) return
  const { dx, dy, stageRect, width, height } = dragState
  const maxX = Math.max(0, stageRect.width - width)
  const maxY = Math.max(0, stageRect.height - height)
  const x = Math.min(maxX, Math.max(0, event.clientX - stageRect.left - dx))
  const y = Math.min(maxY, Math.max(0, event.clientY - stageRect.top - dy))
  toolbarPos.value = { x: Math.round(x), y: Math.round(y) }
}

function onToolbarPointerUp() {
  if (dragState && toolbarPos.value) {
    localStorage.setItem(TOOLBAR_POS_KEY, JSON.stringify(toolbarPos.value))
  }
  dragState = null
  window.removeEventListener('pointermove', onToolbarPointerMove)
  window.removeEventListener('pointerup', onToolbarPointerUp)
  toolbarVisible.value = true
  scheduleHide()
}

function toggleQuality() {
  if (!qualityOpen.value) {
    const bar = toolbarRef.value?.getBoundingClientRect()
    const stage = stageRef.value?.getBoundingClientRect()
    qualityUp.value = !!bar && !!stage && bar.bottom + 200 > stage.bottom
  }
  qualityOpen.value = !qualityOpen.value
}

function selectQuality(key) {
  quality.value = key
  localStorage.setItem(QUALITY_KEY, key)
  qualityOpen.value = false
  applyStreamSettings()
  lastKeyRequestAt = 0
  requestKeyframe()
}

function selectFps(value) {
  fpsSetting.value = value
  localStorage.setItem(FPS_KEY, String(value))
  applyStreamSettings()
}

function onWindowPointerDown(event) {
  if (qualityOpen.value && !event.target.closest?.('.tool-quality-wrap')) {
    qualityOpen.value = false
  }
}

/* ---------- 文件传输 ---------- */

function toggleFilePanel() {
  filePanelOpen.value = !filePanelOpen.value
  qualityOpen.value = false
  if (filePanelOpen.value) {
    toolbarVisible.value = true
    clearHideTimer()
    requestFileList()
    return
  }
  scheduleHide()
}

function requestFileList() {
  send({ event: 'file', action: 'list' })
}

function pickFiles() {
  fileInputRef.value?.click()
}

function onFilesPicked(event) {
  const picked = Array.from(event.target.files || [])
  event.target.value = ''
  for (const file of picked) {
    uploadFile(file)
  }
}

// 逐片上传，控制发送节奏避免把视频通道完全挤占。
async function uploadFile(file) {
  if (!ws || ws.readyState !== WebSocket.OPEN) return
  const id = fileSeq++
  uploads.value.push({ id, name: file.name, size: file.size, sent: 0, done: false, error: '' })
  const item = uploads.value[uploads.value.length - 1]

  send({ event: 'file', action: 'begin', id, name: file.name, size: file.size })
  let offset = 0
  while (offset < file.size) {
    if (!ws || ws.readyState !== WebSocket.OPEN) {
      item.error = lang.t('remote.disconnected')
      return
    }
    const buffer = await file.slice(offset, offset + FILE_CHUNK).arrayBuffer()
    const frame = new Uint8Array(5 + buffer.byteLength)
    frame[0] = TYPE_FILE
    new DataView(frame.buffer).setUint32(1, id, true)
    frame.set(new Uint8Array(buffer), 5)
    ws.send(frame)
    offset += buffer.byteLength
    item.sent = offset
    await new Promise((resolve) => window.setTimeout(resolve, 8))
  }
  send({ event: 'file', action: 'end', id })
}

function requestDownload(file) {
  const id = fileSeq++
  downloading.value = file.name
  send({ event: 'file', action: 'get', id, name: file.name })
}

function handleFileEvent(message) {
  switch (message.action) {
    case 'list':
      remoteFiles.value = Array.isArray(message.files) ? message.files : []
      break
    case 'done': {
      const item = uploads.value.find((entry) => entry.id === message.id)
      if (item) {
        item.done = true
        item.sent = item.size
        if (message.name) item.name = message.name
      }
      requestFileList()
      break
    }
    case 'begin':
      downloads.set(message.id, { name: message.name, chunks: [], received: 0 })
      downloading.value = message.name
      break
    case 'end':
      finishDownload(message.id, message.name)
      break
    case 'error': {
      const item = uploads.value.find((entry) => entry.id === message.id)
      if (item) item.error = message.message || lang.t('remote.fileFailed')
      downloads.delete(message.id)
      downloading.value = ''
      break
    }
  }
}

function finishDownload(id, name) {
  const entry = downloads.get(id)
  downloads.delete(id)
  downloading.value = ''
  if (!entry) return

  const blob = new Blob(entry.chunks, { type: 'application/octet-stream' })
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = name || entry.name
  document.body.appendChild(link)
  link.click()
  link.remove()
  window.setTimeout(() => URL.revokeObjectURL(url), 10000)
}

/* ---------- 剪切板 ---------- */

async function sendClipboard() {
  try {
    const text = await HubService.ReadClipboard()
    if (!text) {
      showHint(lang.t('remote.clipboardEmpty'))
      return
    }
    send({ event: 'clipboard', text })
    showHint(lang.t('remote.clipboardSent'))
  } catch {
    showHint(lang.t('remote.clipboardFailed'))
  }
}

// 把远端剪切板文本写回本机剪切板。
async function applyRemoteClipboard(text) {
  if (!text) return
  try {
    await HubService.WriteClipboard(text)
    showHint(lang.t('remote.clipboardCopied'))
  } catch {
    showHint(lang.t('remote.clipboardFailed'))
  }
}

function showHint(text) {
  clipboardHint.value = text
  if (hintTimer) window.clearTimeout(hintTimer)
  hintTimer = window.setTimeout(() => {
    clipboardHint.value = ''
  }, 2200)
}

function formatSize(size) {
  if (!size) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB']
  let value = size
  let index = 0
  while (value >= 1024 && index < units.length - 1) {
    value /= 1024
    index += 1
  }
  return `${value.toFixed(index === 0 ? 0 : 1)} ${units[index]}`
}

let ws = null
let decoder = null
let decoderConfig = null
let timestamp = 0
let framesThisSecond = 0
let fpsTimer = 0
let watchdogTimer = 0
let lastFrameAt = 0
let lastKeyRequestAt = 0
let moveFrame = 0
let pendingMove = null
let closedByUser = false
const pressedKeys = new Set()

function wsUrl() {
  return typeof route.query.ws === 'string' ? route.query.ws : ''
}

function send(payload) {
  if (ws && ws.readyState === WebSocket.OPEN) {
    ws.send(JSON.stringify(payload))
  }
}

// 把当前画质（最大宽度 + 码率）与帧率同步给被控端，被控端会据此重启编码器。
function applyStreamSettings() {
  const option = currentQuality.value
  send({ event: 'stream', maxWidth: option.maxWidth, bitrate: option.bitrate, fps: fpsSetting.value })
}

function requestKeyframe() {
  const now = Date.now()
  if (now - lastKeyRequestAt < 1500) return
  lastKeyRequestAt = now
  send({ event: 'keyframe' })
}

function setupCanvas(width, height) {
  const canvas = canvasRef.value
  if (!canvas) return
  if (canvas.width !== width) canvas.width = width
  if (canvas.height !== height) canvas.height = height
}

function resetDecoder() {
  if (decoder) {
    try {
      decoder.close()
    } catch {
      // 已关闭时忽略
    }
    decoder = null
  }
  decoderConfig = null
}

// 按分辨率选择 H.264 level，避免 1080p 画面用 level 3.0 配置被解码器拒绝。
function codecString(width, height) {
  const pixels = width * height
  let level = '1e' // 3.0
  if (pixels > 1280 * 720) level = '28' // 4.0
  if (pixels > 1920 * 1088) level = '2a' // 4.2
  return `avc1.42E0${level.toUpperCase()}`
}

function configureDecoder(width, height) {
  if (!supported) return false
  if (decoder && decoderConfig && decoderConfig.width === width && decoderConfig.height === height) {
    return true
  }

  resetDecoder()
  frameSize.value = { width, height }
  setupCanvas(width, height)

  try {
    decoder = new VideoDecoder({
      output: drawFrame,
      error: (err) => {
        console.error('[remote] decoder error', err)
        resetDecoder()
        requestKeyframe()
      }
    })
    // 不传 description，VideoDecoder 按 Annex-B 解析 ffmpeg 的裸码流。
    decoder.configure({
      codec: codecString(width, height),
      optimizeForLatency: true
    })
    decoderConfig = { width, height }
    return true
  } catch (err) {
    console.error('[remote] configure decoder failed', err)
    resetDecoder()
    status.value = 'error'
    errorText.value = lang.t('remote.codecUnsupported')
    return false
  }
}

function drawFrame(frame) {
  const canvas = canvasRef.value
  if (canvas) {
    if (canvas.width !== frame.displayWidth) canvas.width = frame.displayWidth
    if (canvas.height !== frame.displayHeight) canvas.height = frame.displayHeight
    const ctx = canvas.getContext('2d')
    ctx.drawImage(frame, 0, 0, canvas.width, canvas.height)
  }

  frameSize.value = { width: frame.displayWidth, height: frame.displayHeight }
  frame.close()

  framesThisSecond += 1
  lastFrameAt = Date.now()
  if (status.value !== 'streaming') {
    status.value = 'streaming'
    if (settingStore.settings.fullscreenOnConnect) {
      enterFullscreen()
    }
    scheduleHide()
  }
}

// 在 Annex-B 数据里找 SPS(7)/IDR(5)，据此标记关键帧。
function isKeyframe(bytes) {
  for (let i = 0; i + 3 < bytes.length; i++) {
    if (bytes[i] !== 0 || bytes[i + 1] !== 0) continue
    let offset = 0
    if (bytes[i + 2] === 1) {
      offset = 3
    } else if (bytes[i + 2] === 0 && bytes[i + 3] === 1) {
      offset = 4
    } else {
      continue
    }
    const header = bytes[i + offset]
    if (header === undefined) continue
    const nalType = header & 0x1f
    if (nalType === 5 || nalType === 7) return true
  }
  return false
}

function handleBinary(data) {
  const raw = new Uint8Array(data)
  if (raw.length < 1) return

  // 首字节标识消息类型：1=视频访问单元，2=文件分片。
  if (raw[0] === TYPE_FILE) {
    if (raw.length < 5) return
    const id = new DataView(raw.buffer, raw.byteOffset, raw.byteLength).getUint32(1, true)
    const entry = downloads.get(id)
    if (entry) {
      entry.chunks.push(raw.slice(5))
      entry.received += raw.length - 5
    }
    return
  }

  const bytes = raw[0] === TYPE_VIDEO ? raw.subarray(1) : raw
  if (bytes.length < 4) return

  if (!decoder || !decoderConfig) {
    requestKeyframe()
    return
  }

  const key = isKeyframe(bytes)
  // 解码队列积压时丢弃非关键帧，避免延迟越滚越大。
  if (!key && decoder.decodeQueueSize > 12) return

  timestamp += 33333
  try {
    decoder.decode(new EncodedVideoChunk({
      type: key ? 'key' : 'delta',
      timestamp,
      data: bytes
    }))
  } catch (err) {
    console.warn('[remote] decode failed', err)
    resetDecoder()
    requestKeyframe()
  }
}

function handleText(text) {
  try {
    const message = JSON.parse(text)
    if (message.type === 'video') {
      configureDecoder(Number(message.width) || 1280, Number(message.height) || 720)
    } else if (message.type === 'error') {
      status.value = 'error'
      errorText.value = message.message || lang.t('remote.disconnected')
    } else if (message.event === 'file') {
      handleFileEvent(message)
    } else if (message.type === 'clipboard') {
      applyRemoteClipboard(message.text)
    }
  } catch {
    // 忽略非 JSON 文本
  }
}

function connect(url) {
  disconnectSocket()

  if (!supported) {
    status.value = 'error'
    errorText.value = lang.t('remote.codecUnsupported')
    return
  }
  if (!url) {
    status.value = 'error'
    errorText.value = lang.t('remote.noStream')
    return
  }

  closedByUser = false
  status.value = 'connecting'
  errorText.value = ''
  toolbarVisible.value = true
  qualityOpen.value = false

  ws = new WebSocket(url)
  ws.binaryType = 'arraybuffer'

  ws.onopen = () => {
    // 先同步画质设置，再请求关键帧，确保首帧可解码。
    applyStreamSettings()
    requestKeyframe()
  }
  ws.onmessage = (event) => {
    if (typeof event.data === 'string') {
      handleText(event.data)
    } else {
      handleBinary(event.data)
    }
  }
  ws.onerror = () => {
    if (!closedByUser) {
      status.value = 'error'
      errorText.value = lang.t('remote.disconnected')
    }
  }
  ws.onclose = () => {
    if (!closedByUser) {
      status.value = 'error'
      errorText.value = lang.t('remote.disconnected')
    }
  }
}

function disconnectSocket() {
  if (ws) {
    closedByUser = true
    try {
      ws.close(1000)
    } catch {
      // 已关闭时忽略
    }
    ws = null
  }
  resetDecoder()
}

/* ---------- 键鼠输入 ---------- */

function pointerPosition(event) {
  const canvas = canvasRef.value
  if (!canvas) return null
  const rect = canvas.getBoundingClientRect()
  if (!rect.width || !rect.height) return null
  return {
    x: Math.min(1, Math.max(0, (event.clientX - rect.left) / rect.width)),
    y: Math.min(1, Math.max(0, (event.clientY - rect.top) / rect.height))
  }
}

function scheduleMove(event) {
  const point = pointerPosition(event)
  if (!point) return
  pendingMove = point
  if (moveFrame) return
  moveFrame = requestAnimationFrame(() => {
    moveFrame = 0
    if (pendingMove) {
      send({ event: 'move', x: pendingMove.x, y: pendingMove.y })
      pendingMove = null
    }
  })
}

function mouseDown(event) {
  event.preventDefault()
  const point = pointerPosition(event)
  if (point) {
    send({ event: 'move', x: point.x, y: point.y })
  }
  send({ event: 'down', button: event.button })
}

function mouseUp(event) {
  send({ event: 'up', button: event.button })
}

function mouseMove(event) {
  scheduleMove(event)
}

function mouseWheel(event) {
  event.preventDefault()
  send({ event: 'wheel', delta: event.deltaY > 0 ? -120 : 120 })
}

function keyEvent(event, down) {
  if (isShortcutTarget(event)) return
  const vk = event.keyCode
  if (!vk) return
  // 输入法组字过程中的占位按键不转发，避免注入无效的虚拟键。
  if (event.isComposing || vk === 229) return
  event.preventDefault()

  // 带上物理键位，被控端据此注入扫描码，快捷键与按键重复才能和本机一致。
  const payload = { event: down ? 'keydown' : 'keyup', vk, code: event.code || '' }
  if (down) {
    pressedKeys.add(vk)
  } else {
    pressedKeys.delete(vk)
  }
  send(payload)
}

function releaseKeys() {
  for (const vk of pressedKeys) {
    send({ event: 'keyup', vk })
  }
  pressedKeys.clear()
}

function onKeyDown(event) {
  // Ctrl/Cmd+V 用本机剪切板粘贴：直接把文本发给被控端写入并自动粘贴。
  if (isPasteShortcut(event)) {
    event.preventDefault()
    // 松开控制端的 Ctrl，避免被控端注入粘贴后修饰键仍处于按下状态。
    if (pressedKeys.has(17)) {
      pressedKeys.delete(17)
      send({ event: 'keyup', vk: 17 })
    }
    sendClipboard()
    return
  }
  keyEvent(event, true)
}

function onKeyUp(event) {
  if (isPasteShortcut(event)) {
    event.preventDefault()
    return
  }
  keyEvent(event, false)
  // Ctrl/Cmd+C 之后把远端剪切板内容同步回本机。
  if (isCopyShortcut(event)) {
    window.setTimeout(() => send({ event: 'copy' }), 120)
  }
}

function isShortcutTarget(event) {
  return event.target instanceof HTMLInputElement || event.target instanceof HTMLTextAreaElement
}

function isPasteShortcut(event) {
  if (isShortcutTarget(event)) return false
  return (event.ctrlKey || event.metaKey) && !event.shiftKey && !event.altKey && event.keyCode === 86
}

function isCopyShortcut(event) {
  if (isShortcutTarget(event)) return false
  return (event.ctrlKey || event.metaKey) && !event.shiftKey && !event.altKey && event.keyCode === 67
}

function onContextMenu(event) {
  event.preventDefault()
}

/* ---------- 窗口操作 ---------- */

function enterFullscreen() {
  const element = stageRef.value
  if (!element || document.fullscreenElement) return
  element.requestFullscreen?.().catch(() => {
    // 没有用户手势时浏览器会拒绝，忽略即可
  })
}

function toggleFullscreen() {
  if (document.fullscreenElement) {
    document.exitFullscreen?.()
  } else {
    enterFullscreen()
  }
}

function requestDisconnect() {
  if (settingStore.settings.confirmBeforeDisconnect) {
    confirmVisible.value = true
    return
  }
  doDisconnect()
}

function doDisconnect() {
  confirmVisible.value = false
  disconnectSocket()
  releaseKeys()
  router.push(`/remote/${props.id}`)
}

async function reconnect() {
  if (requesting.value) return
  requesting.value = true
  errorText.value = ''
  hub.clearResult()
  try {
    await hub.requestConnect(props.deviceId)
  } catch (err) {
    requesting.value = false
    errorText.value = err?.message ?? String(err)
  }
}

watch(() => hub.lastResult, (result) => {
  if (!result || !requesting.value) return
  requesting.value = false

  if (result.accepted && result.wsUrl) {
    router.replace({ path: route.path, query: { ws: result.wsUrl } })
    return
  }
  errorText.value = result.reason === 'offline' ? lang.t('server.nodeOffline') : lang.t('server.requestRejected')
})

watch(() => route.query.ws, (value) => {
  if (typeof value === 'string' && value) {
    connect(value)
  }
})

onMounted(() => {
  window.addEventListener('keydown', onKeyDown)
  window.addEventListener('keyup', onKeyUp)
  window.addEventListener('blur', releaseKeys)
  window.addEventListener('mouseup', mouseUp)
  window.addEventListener('pointerdown', onWindowPointerDown)

  fpsTimer = window.setInterval(() => {
    fps.value = framesThisSecond
    framesThisSecond = 0
  }, 1000)

  watchdogTimer = window.setInterval(() => {
    if (status.value !== 'streaming') return
    if (Date.now() - lastFrameAt > 2500) {
      requestKeyframe()
    }
  }, 1000)

  connect(wsUrl())
})

onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKeyDown)
  window.removeEventListener('keyup', onKeyUp)
  window.removeEventListener('blur', releaseKeys)
  window.removeEventListener('mouseup', mouseUp)
  window.removeEventListener('pointerdown', onWindowPointerDown)
  window.removeEventListener('pointermove', onToolbarPointerMove)
  window.removeEventListener('pointerup', onToolbarPointerUp)
  clearHideTimer()

  window.clearInterval(fpsTimer)
  window.clearInterval(watchdogTimer)
  if (hintTimer) window.clearTimeout(hintTimer)
  if (moveFrame) cancelAnimationFrame(moveFrame)
  releaseKeys()
  disconnectSocket()
})
</script>

<template>
  <div ref="stageRef" class="stage" :class="{ 'cursor-hidden': status === 'streaming' }">
    <canvas
      ref="canvasRef"
      class="screen"
      tabindex="0"
      @mousedown="mouseDown"
      @mousemove="mouseMove"
      @wheel="mouseWheel"
      @contextmenu="onContextMenu"
    ></canvas>

    <transition name="fade">
      <div v-if="status !== 'streaming'" class="overlay">
        <span v-if="status === 'connecting'" class="overlay-spinner">
          <i class="pi pi-spin pi-spinner"></i>
        </span>
        <span v-else class="overlay-icon">
          <i class="fa-solid fa-triangle-exclamation"></i>
        </span>
        <h2>{{ status === 'connecting' ? lang.t('remote.connecting') : errorText || lang.t('remote.disconnected') }}</h2>
        <p v-if="status === 'connecting'">{{ lang.t('remote.waitingScreen') }}</p>
        <Button
          v-if="status !== 'connecting'"
          :label="lang.t('remote.reconnect')"
          icon="pi pi-refresh"
          :loading="requesting"
          @click="reconnect"
        />
      </div>
    </transition>

    <button
      v-if="!toolbarVisible"
      type="button"
      class="reveal-tab"
      :style="revealStyle"
      :title="lang.t('remote.showToolbar')"
      @pointerenter="revealToolbar"
      @click="revealToolbar"
    >
      <i class="pi pi-bars"></i>
    </button>

    <header
      ref="toolbarRef"
      class="toolbar"
      :class="{ 'is-hidden': !toolbarVisible }"
      :style="toolbarStyle"
      @pointerdown="onToolbarPointerDown"
      @pointerenter="onToolbarEnter"
      @pointerleave="onToolbarLeave"
    >
      <span class="tool-grip" :title="lang.t('remote.dragToolbar')">
        <i class="pi pi-bars"></i>
      </span>

      <button type="button" class="tool-btn" :title="lang.t('remote.back')" @click="requestDisconnect">
        <i class="pi pi-arrow-left"></i>
      </button>

      <div class="tool-device">
        <span class="tool-dot" :class="{ live: status === 'streaming' }"></span>
        <span class="tool-name">{{ deviceName }}</span>
        <span class="tool-server">{{ server?.name }}</span>
      </div>

      <div class="tool-stats">
        <span v-if="frameSize.width">{{ frameSize.width }}×{{ frameSize.height }}</span>
        <span>{{ fps }} {{ lang.t('remote.fps') }}</span>
        <span>H.264</span>
      </div>

      <div class="tool-actions">
        <div class="tool-quality-wrap">
          <button
            type="button"
            class="tool-quality"
            :class="{ open: qualityOpen }"
            :title="lang.t('remote.quality')"
            @click="toggleQuality"
          >
            <span>{{ currentQuality.label }}</span>
            <i class="pi pi-angle-down"></i>
          </button>

          <transition name="fade">
            <div v-if="qualityOpen" class="quality-menu" :class="{ up: qualityUp }">
              <button
                v-for="option in qualityOptions"
                :key="option.key"
                type="button"
                class="quality-item"
                :class="{ active: quality === option.key }"
                @click="selectQuality(option.key)"
              >
                <span class="quality-name">{{ option.label }}</span>
                <span class="quality-hint">{{ option.hint }}</span>
                <i v-if="quality === option.key" class="pi pi-check"></i>
              </button>

              <div class="quality-sep"></div>
              <div class="quality-label">{{ lang.t('remote.frameRate') }}</div>
              <div class="fps-row">
                <button
                  v-for="value in fpsOptions"
                  :key="value"
                  type="button"
                  class="fps-item"
                  :class="{ active: fpsSetting === value }"
                  @click="selectFps(value)"
                >
                  {{ value }}
                </button>
              </div>
            </div>
          </transition>
        </div>

        <button
          type="button"
          class="tool-btn"
          :title="lang.t('remote.sendClipboard')"
          @click="sendClipboard"
        >
          <i class="pi pi-clipboard"></i>
        </button>
        <button
          type="button"
          class="tool-btn"
          :class="{ active: filePanelOpen }"
          :title="lang.t('remote.files')"
          @click="toggleFilePanel"
        >
          <i class="pi pi-folder"></i>
        </button>

        <button
          type="button"
          class="tool-btn"
          :class="{ active: pinned }"
          :title="pinned ? lang.t('remote.unpin') : lang.t('remote.pin')"
          @click="togglePin"
        >
          <i class="pi pi-thumbtack"></i>
        </button>
        <button type="button" class="tool-btn" :title="lang.t('remote.fullscreen')" @click="toggleFullscreen">
          <i class="pi pi-window-maximize"></i>
        </button>
        <button type="button" class="tool-btn" :title="lang.t('remote.reconnect')" @click="reconnect">
          <i class="pi pi-refresh"></i>
        </button>
        <button type="button" class="tool-btn danger" :title="lang.t('remote.disconnect')" @click="requestDisconnect">
          <i class="pi pi-power-off"></i>
        </button>
      </div>
    </header>

    <transition name="fade">
      <section v-if="filePanelOpen" class="file-panel">
        <header class="file-panel-head">
          <span>{{ lang.t('remote.files') }}</span>
          <button type="button" class="file-action" :title="lang.t('remote.close')" @click="toggleFilePanel">
            <i class="pi pi-times"></i>
          </button>
        </header>

        <div class="file-section">
          <div class="file-section-title">
            <span>{{ lang.t('remote.sendFiles') }}</span>
            <button type="button" class="file-action" :title="lang.t('remote.sendFiles')" @click="pickFiles">
              <i class="pi pi-plus"></i>
            </button>
          </div>
          <p class="file-hint">{{ lang.t('remote.fileHint') }}</p>
          <div v-if="uploads.length" class="file-list">
            <div v-for="item in uploads" :key="item.id" class="file-row">
              <span class="file-name" :title="item.name">{{ item.name }}</span>
              <span v-if="item.error" class="file-error">{{ item.error }}</span>
              <span v-else-if="item.done" class="file-done"><i class="pi pi-check"></i></span>
              <span v-else class="file-progress">
                {{ Math.floor((item.sent / Math.max(1, item.size)) * 100) }}%
              </span>
            </div>
          </div>
        </div>

        <div class="file-section">
          <div class="file-section-title">
            <span>{{ lang.t('remote.remoteFiles') }}</span>
            <button type="button" class="file-action" :title="lang.t('remote.refresh')" @click="requestFileList">
              <i class="pi pi-refresh"></i>
            </button>
          </div>
          <p v-if="!remoteFiles.length" class="file-hint">{{ lang.t('remote.emptyFiles') }}</p>
          <div v-else class="file-list">
            <div v-for="file in remoteFiles" :key="file.name" class="file-row">
              <span class="file-name" :title="file.name">{{ file.name }}</span>
              <span class="file-size">{{ formatSize(file.size) }}</span>
              <button
                type="button"
                class="file-action"
                :title="lang.t('remote.download')"
                @click="requestDownload(file)"
              >
                <i class="pi pi-download"></i>
              </button>
            </div>
          </div>
          <p v-if="downloading" class="file-hint">{{ lang.t('remote.downloading') }} {{ downloading }}</p>
        </div>

        <input ref="fileInputRef" type="file" multiple class="file-input" @change="onFilesPicked" />
      </section>
    </transition>

    <transition name="fade">
      <div v-if="clipboardHint" class="stage-toast">{{ clipboardHint }}</div>
    </transition>

    <transition name="fade">
      <div v-if="confirmVisible" class="confirm-mask" @click.self="confirmVisible = false">
        <div class="confirm-card">
          <h3>{{ lang.t('remote.disconnect') }}</h3>
          <p class="confirm-text">{{ lang.t('settings.confirmBeforeDisconnectHint') }}</p>
          <div class="confirm-actions">
            <Button :label="lang.t('home.cancel')" severity="secondary" text @click="confirmVisible = false" />
            <Button :label="lang.t('remote.disconnect')" severity="danger" @click="doDisconnect" />
          </div>
        </div>
      </div>
    </transition>
  </div>
</template>

<style scoped>
.stage {
  position: relative;
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  overflow: hidden;
  background: #000;
  outline: none;
}

.screen {
  max-width: 100%;
  max-height: 100%;
  width: auto;
  height: auto;
  display: block;
  outline: none;
  background: #000;
  image-rendering: auto;
}

/* 远程画面里隐藏本地光标，避免和远端光标重叠。 */
.stage.cursor-hidden,
.stage.cursor-hidden .screen {
  cursor: none;
}

/* 工具栏和面板保持可用，但不影响画面的光标隐藏。 */
.stage.cursor-hidden .toolbar,
.stage.cursor-hidden .file-panel,
.stage.cursor-hidden .confirm-card {
  cursor: default;
}

.toolbar {
  position: absolute;
  top: 14px;
  right: 14px;
  display: flex;
  align-items: center;
  gap: 14px;
  max-width: calc(100% - 28px);
  padding: 8px 10px 8px 10px;
  border-radius: 12px;
  border: 1px solid rgba(255, 255, 255, 0.08);
  background: rgba(21, 25, 32, 0.88);
  backdrop-filter: blur(12px);
  box-shadow: 0 12px 32px rgba(0, 0, 0, 0.45);
  z-index: 5;
  touch-action: none;
  transition: opacity 0.2s ease;
}

.toolbar.is-hidden {
  opacity: 0;
  pointer-events: none;
}

.tool-grip {
  display: grid;
  place-items: center;
  width: 14px;
  height: 32px;
  margin-left: -2px;
  font-size: 10px;
  color: var(--app-text-dim);
  cursor: grab;
  user-select: none;
}

.toolbar:active .tool-grip {
  cursor: grabbing;
}

.reveal-tab {
  position: absolute;
  top: 10px;
  right: 14px;
  width: 46px;
  height: 18px;
  display: grid;
  place-items: center;
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 0 0 8px 8px;
  border-top: none;
  background: rgba(21, 25, 32, 0.6);
  color: var(--app-text-dim);
  font-size: 9px;
  cursor: pointer;
  opacity: 0.55;
  z-index: 5;
  transition: opacity 0.15s ease;
}

.reveal-tab:hover {
  opacity: 1;
}

.tool-btn {
  width: 32px;
  height: 32px;
  display: grid;
  place-items: center;
  border: none;
  border-radius: 9px;
  font-size: 13px;
  color: var(--app-text-muted);
  background: transparent;
  cursor: pointer;
}

.tool-btn:hover {
  color: var(--app-text);
  background: rgba(255, 255, 255, 0.08);
}

.tool-btn.danger:hover {
  color: #ff8a87;
  background: rgba(239, 83, 80, 0.16);
}

.tool-btn.active {
  color: var(--app-accent);
  background: rgba(47, 124, 246, 0.16);
}

.tool-quality-wrap {
  position: relative;
}

.tool-quality {
  display: flex;
  align-items: center;
  gap: 5px;
  height: 32px;
  padding: 0 9px;
  border: none;
  border-radius: 9px;
  font-size: 12px;
  color: var(--app-text-muted);
  background: transparent;
  cursor: pointer;
  white-space: nowrap;
}

.tool-quality:hover,
.tool-quality.open {
  color: var(--app-text);
  background: rgba(255, 255, 255, 0.08);
}

.tool-quality i {
  font-size: 9px;
}

.quality-menu {
  position: absolute;
  top: calc(100% + 10px);
  right: 0;
  min-width: 178px;
  padding: 6px;
  display: flex;
  flex-direction: column;
  gap: 2px;
  border-radius: 10px;
  border: 1px solid rgba(255, 255, 255, 0.08);
  background: rgba(21, 25, 32, 0.97);
  backdrop-filter: blur(12px);
  box-shadow: 0 16px 36px rgba(0, 0, 0, 0.5);
  z-index: 6;
}

.quality-menu.up {
  top: auto;
  bottom: calc(100% + 10px);
}

.quality-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 7px 9px;
  border: none;
  border-radius: 7px;
  background: transparent;
  color: var(--app-text-muted);
  font-size: 12px;
  text-align: left;
  cursor: pointer;
}

.quality-item:hover {
  color: var(--app-text);
  background: rgba(255, 255, 255, 0.07);
}

.quality-item.active {
  color: var(--app-text);
  background: rgba(47, 124, 246, 0.16);
}

.quality-name {
  flex: 1;
}

.quality-hint {
  font-size: 11px;
  color: var(--app-text-dim);
}

.quality-item i {
  font-size: 10px;
  color: var(--app-accent);
}

.quality-sep {
  height: 1px;
  margin: 4px 2px;
  background: rgba(255, 255, 255, 0.08);
}

.quality-label {
  padding: 2px 9px 4px;
  font-size: 11px;
  color: var(--app-text-dim);
}

.fps-row {
  display: flex;
  gap: 4px;
  padding: 0 4px 2px;
}

.fps-item {
  flex: 1;
  height: 26px;
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 7px;
  background: transparent;
  color: var(--app-text-muted);
  font-size: 12px;
  cursor: pointer;
}

.fps-item:hover {
  color: var(--app-text);
  background: rgba(255, 255, 255, 0.07);
}

.fps-item.active {
  color: var(--app-text);
  border-color: var(--app-accent);
  background: rgba(47, 124, 246, 0.16);
}

.tool-device {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.tool-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: #6b7280;
}

.tool-dot.live {
  background: var(--app-success);
  box-shadow: 0 0 0 3px rgba(47, 191, 113, 0.18);
}

.tool-name {
  font-size: 13px;
  font-weight: 500;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.tool-server {
  font-size: 11px;
  color: var(--app-text-dim);
  white-space: nowrap;
}

.tool-stats {
  display: flex;
  align-items: center;
  gap: 10px;
  padding-left: 12px;
  border-left: 1px solid rgba(255, 255, 255, 0.1);
  font-size: 11px;
  color: var(--app-text-muted);
  white-space: nowrap;
}

.tool-actions {
  display: flex;
  align-items: center;
  gap: 2px;
  padding-left: 8px;
  border-left: 1px solid rgba(255, 255, 255, 0.1);
}

/* ---------- 文件面板 ---------- */

.file-panel {
  position: absolute;
  top: 68px;
  right: 14px;
  width: 320px;
  max-height: calc(100% - 90px);
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 12px;
  border-radius: 12px;
  border: 1px solid rgba(255, 255, 255, 0.08);
  background: rgba(21, 25, 32, 0.96);
  backdrop-filter: blur(12px);
  box-shadow: 0 16px 40px rgba(0, 0, 0, 0.5);
  overflow: auto;
  z-index: 7;
}

.file-panel-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 13px;
  font-weight: 500;
}

.file-section {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding-top: 8px;
  border-top: 1px solid rgba(255, 255, 255, 0.08);
}

.file-section-title {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 12px;
  color: var(--app-text-muted);
}

.file-hint {
  font-size: 11px;
  line-height: 1.5;
  color: var(--app-text-dim);
  word-break: break-all;
}

.file-list {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.file-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 5px 4px;
  border-radius: 7px;
  font-size: 12px;
}

.file-row:hover {
  background: rgba(255, 255, 255, 0.05);
}

.file-name {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.file-size,
.file-progress {
  font-size: 11px;
  color: var(--app-text-dim);
}

.file-done {
  font-size: 11px;
  color: var(--app-success);
}

.file-error {
  font-size: 11px;
  color: #ff8a87;
}

.file-action {
  display: grid;
  place-items: center;
  width: 24px;
  height: 24px;
  flex: 0 0 auto;
  border: none;
  border-radius: 6px;
  background: transparent;
  color: var(--app-text-muted);
  font-size: 11px;
  cursor: pointer;
}

.file-action:hover {
  color: var(--app-text);
  background: rgba(255, 255, 255, 0.08);
}

.file-input {
  display: none;
}

.stage-toast {
  position: absolute;
  left: 50%;
  bottom: 32px;
  transform: translateX(-50%);
  padding: 8px 16px;
  border-radius: 8px;
  background: rgba(21, 25, 32, 0.92);
  border: 1px solid rgba(255, 255, 255, 0.08);
  color: var(--app-text);
  font-size: 12px;
  z-index: 8;
  pointer-events: none;
}

/* ---------- 断开确认 ---------- */

.confirm-mask {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(0, 0, 0, 0.55);
  z-index: 20;
}

.confirm-card {
  width: 380px;
  max-width: calc(100% - 40px);
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 20px;
  border-radius: 14px;
  border: 1px solid rgba(255, 255, 255, 0.08);
  background: #151920;
  box-shadow: 0 20px 48px rgba(0, 0, 0, 0.6);
}

.confirm-card h3 {
  font-size: 15px;
  font-weight: 500;
  color: var(--app-text);
}

.confirm-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}

.overlay {
  position: absolute;
  inset: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 10px;
  background: rgba(8, 10, 14, 0.72);
  text-align: center;
  z-index: 4;
}

.overlay-spinner {
  font-size: 28px;
  color: var(--app-accent);
}

.overlay-icon {
  font-size: 26px;
  color: var(--app-danger);
}

.overlay h2 {
  font-size: 15px;
  font-weight: 500;
  color: var(--app-text);
}

.overlay p {
  font-size: 12px;
  color: var(--app-text-muted);
}

.overlay :deep(.p-button) {
  margin-top: 6px;
}

.confirm-text {
  font-size: 13px;
  line-height: 1.6;
  color: var(--app-text-muted);
}

.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.18s ease;
}

.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
