import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import { Events } from '@wailsio/runtime'
import { HubService } from '../../bindings/changeme'

export interface HubDevice {
  deviceId: string
  name: string
  os: string
}

export interface ConnectResult {
  requestId: string
  accepted: boolean
  wsUrl?: string
  reason?: string
}

function ensureDeviceId() {
  const stored = localStorage.getItem('openremote.deviceId')
  if (stored) return stored
  const id = (globalThis.crypto?.randomUUID?.() ?? `dev-${Date.now().toString(36)}`).replace(/-/g, '')
  localStorage.setItem('openremote.deviceId', id)
  return id
}

export const useHubStore = defineStore('hub', () => {
  const localId = ref(ensureDeviceId())
  const deviceName = ref(localStorage.getItem('openremote.deviceName') || '我的电脑')

  const nodeUrl = ref(localStorage.getItem('openremote.nodeUrl') || '')
  const connected = ref(false)
  const connecting = ref(false)
  const error = ref('')
  const devices = ref<HubDevice[]>([])
  const lastResult = ref<ConnectResult | null>(null)
  const pendingTargetId = ref('')

  const peers = computed(() => devices.value.filter((item) => item.deviceId !== localId.value))
  const localDevice = computed(() => devices.value.find((item) => item.deviceId === localId.value))

  let offDevices: (() => void) | null = null
  let offResult: (() => void) | null = null
  let offDisconnected: (() => void) | null = null
  let offError: (() => void) | null = null

  function bindEvents() {
    if (offDevices) return
    offDevices = Events.On('hub:devices', (event) => {
      devices.value = event.data ?? []
    })
    offResult = Events.On('hub:connect-result', (event) => {
      lastResult.value = event.data ?? null
    })
    offDisconnected = Events.On('hub:disconnected', () => {
      connected.value = false
    })
    offError = Events.On('hub:error', (event) => {
      const data = event.data as { message?: string } | undefined
      if (data?.message) error.value = data.message
    })
  }

  function saveDeviceName(name: string) {
    deviceName.value = name.trim() || '我的电脑'
    localStorage.setItem('openremote.deviceName', deviceName.value)
  }

  async function connectTo(url: string) {
    bindEvents()
    if (!url) {
      error.value = '未配置节点地址'
      return false
    }
    if (connected.value && nodeUrl.value === url) {
      return true
    }
    connecting.value = true
    error.value = ''
    try {
      await HubService.Connect(localId.value, deviceName.value, url)
      nodeUrl.value = url
      localStorage.setItem('openremote.nodeUrl', url)
      connected.value = true
      return true
    } catch (err) {
      connected.value = false
      error.value = err?.message ?? String(err)
      return false
    } finally {
      connecting.value = false
    }
  }

  async function disconnect() {
    try {
      await HubService.Disconnect()
    } catch {
      // 未连接时忽略
    }
    connected.value = false
    devices.value = []
  }

  // reconnect 用当前设备名称重新注册，让其他设备看到新的名字。
  async function reconnect() {
    const url = nodeUrl.value
    if (!url) return false
    await disconnect()
    return connectTo(url)
  }

  // requestConnect 发起远程控制请求；节点返回的 wsUrl 由 lastResult 交给控制页。
  async function requestConnect(targetId: string) {
    pendingTargetId.value = targetId
    lastResult.value = null
    return HubService.RequestConnect(targetId)
  }

  function clearResult() {
    lastResult.value = null
    pendingTargetId.value = ''
  }

  return {
    localId,
    deviceName,
    nodeUrl,
    connected,
    connecting,
    error,
    devices,
    peers,
    localDevice,
    lastResult,
    pendingTargetId,
    saveDeviceName,
    connectTo,
    disconnect,
    reconnect,
    requestConnect,
    clearResult
  }
})
