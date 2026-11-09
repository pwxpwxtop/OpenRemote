<script setup>
import { computed, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import { HubService } from '../../../bindings/changeme'
import { useServerStore } from '../../stores/servers'
import { useHubStore } from '../../stores/hub'
import { useSettingStore } from '../../stores/SettingConf'
import { useLanguageStore } from '../../stores/Language'

const props = defineProps({
  id: {
    type: String,
    default: undefined
  }
})

const router = useRouter()
const serverStore = useServerStore()
const hub = useHubStore()
const settingStore = useSettingStore()
const lang = useLanguageStore()

const confirmVisible = ref(false)
const waitTargetId = ref('')
const launchError = ref('')

const server = computed(() => serverStore.getById(props.id))
const notFound = computed(() => !server.value)
const hasNode = computed(() => Boolean(server.value?.nodeUrl))
const devices = computed(() => hub.peers)

const status = computed(() => {
  if (!hasNode.value) return 'missing'
  if (hub.connecting) return 'connecting'
  if (hub.connected) return 'connected'
  return 'disconnected'
})

const statusLabel = computed(() => {
  switch (status.value) {
    case 'connected':
      return lang.t('remote.connected')
    case 'connecting':
      return lang.t('server.connectingNode')
    case 'missing':
      return lang.t('server.nodeMissing')
    default:
      return lang.t('server.nodeOffline')
  }
})

function formatAddress(target) {
  if (!target) return ''
  return target.port ? `${target.host}:${target.port}` : target.host
}

function osIcon(os) {
  switch (os) {
    case 'windows':
      return 'fa-windows'
    case 'darwin':
      return 'fa-apple'
    default:
      return 'fa-linux'
  }
}

function osLabel(os) {
  switch (os) {
    case 'windows':
      return 'Windows'
    case 'darwin':
      return 'macOS'
    case 'linux':
      return 'Linux'
    default:
      return os || 'Unknown'
  }
}

async function connectNode() {
  const current = server.value
  if (!current) return

  hub.clearResult()
  await hub.disconnect()

  let url = current.nodeUrl || ''

  // 重连时顺带检查服务器上的中转节点，进程挂了会重新拉起，没装则自动安装。
  if (current.password) {
    try {
      const ensured = await HubService.EnsureNode(
        current.host,
        current.port,
        current.username || 'root',
        current.password,
        9000
      )
      if (ensured) {
        url = ensured
        if (current.nodeUrl !== ensured) {
          serverStore.updateServer(current.id, { nodeUrl: ensured })
        }
      }
    } catch {
      // 部署失败时仍用已保存的地址尝试连接
    }
  }

  if (url) {
    await hub.connectTo(url)
  }
}

async function control(device) {
  launchError.value = ''
  waitTargetId.value = device.deviceId
  try {
    await hub.requestConnect(device.deviceId)
  } catch (err) {
    waitTargetId.value = ''
    launchError.value = err?.message ?? String(err)
  }
}

function requestDisconnect() {
  if (settingStore.settings.confirmBeforeDisconnect && status.value === 'connected') {
    confirmVisible.value = true
    return
  }
  doDisconnect()
}

async function doDisconnect() {
  confirmVisible.value = false
  await hub.disconnect()
}

function editServer() {
  router.push(`/servers/${props.id}/edit`)
}

function back() {
  router.push('/')
}

watch(() => hub.lastResult, (result) => {
  if (!result) return

  if (!result.accepted) {
    waitTargetId.value = ''
    launchError.value = result.reason === 'offline'
      ? lang.t('server.nodeOffline')
      : lang.t('server.requestRejected')
    hub.clearResult()
    return
  }

  const targetId = hub.pendingTargetId
  if (!targetId || !result.wsUrl) return

  waitTargetId.value = ''
  router.push({
    path: `/remote/${props.id}/device/${targetId}`,
    query: { ws: result.wsUrl }
  })
})

onMounted(async () => {
  if (notFound.value) return
  serverStore.markConnected(props.id)
  if (server.value?.nodeUrl) {
    await hub.connectTo(server.value.nodeUrl)
  }
})
</script>

<template>
  <div class="page">
    <header class="page-head">
      <button type="button" class="back-btn" @click="back">
        <i class="pi pi-arrow-left"></i>
        {{ lang.t('server.back') }}
      </button>

      <div class="server-title">
        <span class="server-icon"><i class="fa-solid fa-server"></i></span>
        <div>
          <p class="server-name">{{ server?.name ?? lang.t('remote.notFound') }}</p>
          <p class="server-address">{{ server?.nodeUrl || formatAddress(server) }}</p>
        </div>
      </div>

      <span class="status-pill" :class="status">{{ statusLabel }}</span>

      <div class="head-actions">
        <Button
          v-if="status === 'connected'"
          icon="pi pi-refresh"
          :label="lang.t('server.refresh')"
          text
          severity="secondary"
          @click="connectNode"
        />
        <Button
          v-if="status === 'connected'"
          icon="pi pi-power-off"
          :label="lang.t('server.disconnect')"
          severity="danger"
          @click="requestDisconnect"
        />
        <Button
          v-else
          icon="pi pi-replay"
          :label="lang.t('remote.reconnect')"
          :disabled="!hasNode"
          @click="connectNode"
        />
      </div>
    </header>

    <main class="body">
      <div v-if="notFound" class="state">
        <span class="state-icon danger"><i class="fa-solid fa-circle-exclamation"></i></span>
        <h2>{{ lang.t('remote.notFound') }}</h2>
        <p>{{ lang.t('remote.notFoundHint') }}</p>
        <Button :label="lang.t('server.back')" icon="pi pi-arrow-left" @click="back" />
      </div>

      <div v-else-if="!hasNode" class="state">
        <span class="state-icon danger"><i class="fa-solid fa-plug-circle-xmark"></i></span>
        <h2>{{ lang.t('server.nodeMissing') }}</h2>
        <p>{{ lang.t('server.nodeMissingHint') }}</p>
        <Button :label="lang.t('home.edit')" icon="pi pi-pencil" @click="editServer" />
      </div>

      <template v-else>
        <div class="section-head">
          <h2>
            {{ lang.t('server.onlineComputers') }}
            <span class="section-count">{{ devices.length }}</span>
          </h2>
          <span v-if="waitTargetId" class="waiting">
            <i class="pi pi-spin pi-spinner"></i>
            {{ lang.t('server.waitingApproval') }}
          </span>
        </div>

        <p v-if="launchError" class="error-line">
          <i class="pi pi-exclamation-circle"></i>
          {{ launchError }}
        </p>

        <div v-if="status !== 'connected'" class="state">
          <span class="state-icon" :class="{ active: status === 'connecting' }">
            <i v-if="status === 'connecting'" class="pi pi-spin pi-spinner"></i>
            <i v-else class="pi pi-power-off"></i>
          </span>
          <h2>{{ statusLabel }}</h2>
          <p>{{ hub.error || server?.nodeUrl }}</p>
          <Button
            v-if="status !== 'connecting'"
            :label="lang.t('remote.reconnect')"
            icon="pi pi-refresh"
            @click="connectNode"
          />
        </div>

        <div v-else-if="devices.length" class="device-list">
          <article v-for="device in devices" :key="device.deviceId" class="device-row">
            <span class="device-icon"><i :class="['fa-brands', osIcon(device.os)]"></i></span>
            <div class="device-main">
              <p class="device-name">{{ device.name }}</p>
              <p class="device-meta">
                {{ osLabel(device.os) }} · {{ device.deviceId.slice(0, 10) }}
              </p>
            </div>
            <span class="online-badge">
              <i class="fa-solid fa-circle"></i>
              {{ lang.t('server.online') }}
            </span>
            <Button
              :label="lang.t('server.remoteControl')"
              icon="pi pi-desktop"
              size="small"
              :loading="waitTargetId === device.deviceId"
              :disabled="Boolean(waitTargetId)"
              @click="control(device)"
            />
          </article>
        </div>

        <div v-else class="state">
          <span class="state-icon"><i class="fa-solid fa-desktop"></i></span>
          <h2>{{ lang.t('server.noDevices') }}</h2>
          <p>{{ lang.t('server.noDevicesHint') }}</p>
          <p v-if="hub.localDevice" class="self-online">
            <i class="fa-solid fa-circle-check"></i>
            {{ lang.t('server.selfOnline', { name: hub.localDevice.name }) }}
          </p>
        </div>
      </template>
    </main>

    <Dialog
      v-model:visible="confirmVisible"
      modal
      :header="lang.t('remote.disconnect')"
      :style="{ width: '380px' }"
    >
      <p class="confirm-text">{{ lang.t('settings.confirmBeforeDisconnectHint') }}</p>
      <template #footer>
        <Button :label="lang.t('home.cancel')" severity="secondary" text @click="confirmVisible = false" />
        <Button :label="lang.t('remote.disconnect')" severity="danger" @click="doDisconnect" />
      </template>
    </Dialog>
  </div>
</template>

<style scoped>
.page {
  height: 100%;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.page-head {
  display: flex;
  align-items: center;
  gap: 18px;
  padding: 18px 28px;
  border-bottom: 1px solid var(--app-border);
  background: var(--app-sidebar-bg);
}

.back-btn {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  padding: 7px 12px;
  border: 1px solid var(--app-border);
  border-radius: 9px;
  font-size: 12px;
  color: var(--app-text-muted);
  background: transparent;
  cursor: pointer;
}

.back-btn:hover {
  color: var(--app-text);
  border-color: var(--app-border-strong);
}

.server-title {
  display: flex;
  align-items: center;
  gap: 12px;
  min-width: 0;
}

.server-icon {
  width: 38px;
  height: 38px;
  display: grid;
  place-items: center;
  border-radius: 10px;
  color: var(--app-accent);
  background: var(--app-icon-bg);
}

.server-name {
  font-size: 15px;
  font-weight: 600;
}

.server-address {
  font-size: 11px;
  font-family: Consolas, monospace;
  color: var(--app-text-dim);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.status-pill {
  padding: 4px 10px;
  border-radius: 999px;
  font-size: 11px;
  background: rgba(148, 163, 184, 0.12);
  color: var(--app-text-muted);
}

.status-pill.connected {
  color: #6fd39b;
  background: rgba(47, 191, 113, 0.14);
}

.status-pill.connecting {
  color: #f0b45c;
  background: rgba(245, 165, 36, 0.14);
}

.head-actions {
  margin-left: auto;
  display: flex;
  align-items: center;
  gap: 8px;
}

.body {
  flex: 1;
  overflow-y: auto;
  padding: 26px 28px 36px;
}

.section-head {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 14px;
}

.section-head h2 {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 15px;
  font-weight: 600;
}

.section-count {
  min-width: 22px;
  height: 20px;
  padding: 0 7px;
  display: inline-grid;
  place-items: center;
  border-radius: 999px;
  font-size: 11px;
  color: var(--app-text-muted);
  background: var(--app-surface);
}

.waiting {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--app-warning);
}

.error-line {
  display: flex;
  align-items: center;
  gap: 7px;
  margin-bottom: 12px;
  font-size: 12px;
  color: var(--app-danger);
}

.device-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.device-row {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 14px 16px;
  border-radius: 12px;
  border: 1px solid var(--app-border);
  background: var(--app-surface);
}

.device-row:hover {
  border-color: var(--app-border-strong);
}

.device-icon {
  width: 40px;
  height: 40px;
  flex: 0 0 40px;
  display: grid;
  place-items: center;
  border-radius: 11px;
  font-size: 17px;
  color: var(--app-accent);
  background: var(--app-icon-bg);
}

.device-main {
  flex: 1;
  min-width: 0;
}

.device-name {
  font-size: 14px;
  font-weight: 500;
}

.device-meta {
  margin-top: 2px;
  font-size: 11px;
  color: var(--app-text-muted);
}

.online-badge {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  font-size: 11px;
  color: #6fd39b;
}

.online-badge i {
  font-size: 6px;
}

.state {
  display: flex;
  flex-direction: column;
  align-items: center;
  text-align: center;
  padding: 60px 24px;
  border-radius: 16px;
  border: 1px dashed var(--app-border);
  background: var(--app-surface-soft);
}

.state-icon {
  width: 58px;
  height: 58px;
  margin-bottom: 14px;
  display: grid;
  place-items: center;
  border-radius: 16px;
  font-size: 22px;
  color: var(--app-accent);
  background: var(--app-icon-bg);
}

.state-icon.danger {
  color: var(--app-danger);
  background: rgba(239, 83, 80, 0.12);
}

.state-icon.active {
  color: var(--app-warning);
  background: rgba(245, 165, 36, 0.12);
}

.state h2 {
  margin-bottom: 6px;
  font-size: 15px;
  font-weight: 600;
}

.state p {
  margin-bottom: 18px;
  font-size: 13px;
  color: var(--app-text-muted);
}

.self-online {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  margin: 0;
  font-size: 12px;
  color: #6fd39b;
}

.confirm-text {
  font-size: 13px;
  line-height: 1.6;
  color: var(--app-text-muted);
}
</style>
