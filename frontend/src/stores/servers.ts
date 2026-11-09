import { computed, ref, watch } from 'vue'
import { defineStore } from 'pinia'

export interface ServerConfig {
  id: string
  name: string
  host: string
  port: number
  username: string
  password: string
  /** 通过 SSH 部署后得到的转发节点地址，例如 ws://10.0.0.5:9000/ws */
  nodeUrl?: string
  remark: string
  createdAt: number
  lastConnectedAt: number | null
}

export type ServerInput = Omit<ServerConfig, 'id' | 'createdAt' | 'lastConnectedAt'>

const STORAGE_KEY = 'openremote.servers'

function createId(): string {
  return `srv_${Date.now().toString(36)}${Math.random().toString(36).slice(2, 8)}`
}

function loadServers(): ServerConfig[] {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (raw) {
      const parsed = JSON.parse(raw)
      if (Array.isArray(parsed)) return parsed as ServerConfig[]
    }
  } catch {
    // 数据损坏时按空列表处理
  }
  return []
}

export const useServerStore = defineStore('servers', () => {
  const servers = ref<ServerConfig[]>(loadServers())

  watch(servers, (value) => {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(value))
  }, { deep: true })

  const serverCount = computed(() => servers.value.length)

  const recentServers = computed(() =>
    [...servers.value]
      .filter((server) => server.lastConnectedAt !== null)
      .sort((a, b) => (b.lastConnectedAt ?? 0) - (a.lastConnectedAt ?? 0))
  )

  const lastConnectedServer = computed(() => recentServers.value[0] ?? null)

  function getById(id?: string): ServerConfig | null {
    if (!id) return null
    return servers.value.find((server) => server.id === id) ?? null
  }

  function addServer(input: ServerInput): ServerConfig {
    const server: ServerConfig = {
      ...input,
      id: createId(),
      createdAt: Date.now(),
      lastConnectedAt: null
    }
    servers.value.push(server)
    return server
  }

  function updateServer(id: string, patch: Partial<ServerInput>) {
    const server = getById(id)
    if (!server) return
    Object.assign(server, patch)
  }

  function removeServer(id: string) {
    servers.value = servers.value.filter((server) => server.id !== id)
  }

  function markConnected(id: string) {
    const server = getById(id)
    if (server) server.lastConnectedAt = Date.now()
  }

  return {
    servers,
    serverCount,
    recentServers,
    lastConnectedServer,
    getById,
    addServer,
    updateServer,
    removeServer,
    markConnected
  }
})
