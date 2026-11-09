<script setup>
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import { useServerStore } from '../../stores/servers'
import { useHubStore } from '../../stores/hub'
import { useLanguageStore } from '../../stores/Language'

const router = useRouter()
const serverStore = useServerStore()
const hub = useHubStore()
const lang = useLanguageStore()

const deleteVisible = ref(false)
const deleteTarget = ref(null)
const copied = ref(false)

const deviceIdLabel = computed(() => {
  const raw = hub.localId.replace(/-/g, '').toUpperCase()
  return raw.slice(0, 12).replace(/(.{4})/g, '$1 ').trim()
})

async function copyId() {
  try {
    await navigator.clipboard.writeText(hub.localId)
    copied.value = true
    setTimeout(() => { copied.value = false }, 1500)
  } catch {
    // 剪贴板不可用时忽略
  }
}

function formatAddress(server) {
  return server.port ? `${server.host}:${server.port}` : server.host
}

function formatTime(timestamp) {
  if (!timestamp) return lang.t('home.neverConnected')
  const locale = lang.isZh ? 'zh-CN' : 'en-US'
  return new Intl.DateTimeFormat(locale, {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit'
  }).format(new Date(timestamp))
}

function connect(server) {
  router.push(`/remote/${server.id}`)
}

function edit(server) {
  router.push(`/servers/${server.id}/edit`)
}

function askDelete(server) {
  deleteTarget.value = server
  deleteVisible.value = true
}

function confirmDelete() {
  if (deleteTarget.value) {
    serverStore.removeServer(deleteTarget.value.id)
  }
  deleteVisible.value = false
  deleteTarget.value = null
}
</script>

<template>
  <div class="page">
    <header class="page-head">
      <div>
        <h1>{{ lang.t('home.title') }}</h1>
        <p>{{ lang.t('home.subtitle') }}</p>
      </div>
      <Button :label="lang.t('nav.addServer')" icon="pi pi-plus" @click="router.push('/servers/add')" />
    </header>

    <section class="identity">
      <div class="identity-main">
        <span class="identity-label">{{ lang.t('home.myDevice') }}</span>
        <div class="identity-id">
          <span class="id-value">{{ deviceIdLabel }}</span>
          <button type="button" class="copy-btn" :title="lang.t('home.copy')" @click="copyId">
            <i :class="copied ? 'pi pi-check' : 'pi pi-copy'"></i>
          </button>
        </div>
        <p class="identity-name">
          {{ hub.deviceName }}
          <span v-if="copied" class="copied">{{ lang.t('home.copied') }}</span>
        </p>
      </div>

      <div class="identity-status">
        <span class="status-dot" :class="{ online: hub.connected }"></span>
        <div>
          <p class="status-title">{{ lang.t('home.node') }}</p>
          <p class="status-value">
            {{ hub.connected ? lang.t('home.nodeConnected') : lang.t('home.nodeOffline') }}
          </p>
          <p class="status-url">{{ hub.nodeUrl || '—' }}</p>
        </div>
      </div>
    </section>

    <p class="hint">
      <i class="pi pi-info-circle"></i>
      {{ lang.t('home.hint') }}
    </p>

    <section class="servers">
      <div class="section-head">
        <h2>{{ lang.t('home.servers') }}</h2>
        <span class="section-count">{{ serverStore.serverCount }}</span>
      </div>

      <div v-if="serverStore.serverCount" class="server-list">
        <article v-for="server in serverStore.servers" :key="server.id" class="server-row">
          <span class="row-icon"><i class="fa-solid fa-server"></i></span>
          <div class="row-main">
            <p class="row-name">{{ server.name }}</p>
            <p class="row-sub">
              {{ formatAddress(server) }} · {{ formatTime(server.lastConnectedAt) }}
            </p>
          </div>
          <span class="row-badge" :class="{ online: Boolean(server.nodeUrl) }">
            {{ server.nodeUrl ? lang.t('home.nodeConnected') : lang.t('home.noNode') }}
          </span>
          <div class="row-actions">
            <Button icon="pi pi-pencil" text rounded size="small" severity="secondary" :aria-label="lang.t('home.edit')" @click="edit(server)" />
            <Button icon="pi pi-trash" text rounded size="small" severity="danger" :aria-label="lang.t('home.delete')" @click="askDelete(server)" />
            <Button :label="lang.t('home.connect')" icon="pi pi-angle-right" icon-pos="right" size="small" @click="connect(server)" />
          </div>
        </article>
      </div>

      <div v-else class="empty">
        <span class="empty-icon"><i class="fa-solid fa-server"></i></span>
        <h3>{{ lang.t('home.empty') }}</h3>
        <p>{{ lang.t('home.emptyHint') }}</p>
        <Button :label="lang.t('home.addFirst')" icon="pi pi-plus" @click="router.push('/servers/add')" />
      </div>
    </section>

    <Dialog
      v-model:visible="deleteVisible"
      modal
      :header="lang.t('home.deleteConfirmTitle')"
      :style="{ width: '400px' }"
    >
      <p class="delete-message">
        {{ lang.t('home.deleteConfirmMessage', { name: deleteTarget?.name ?? '' }) }}
      </p>
      <template #footer>
        <Button :label="lang.t('home.cancel')" severity="secondary" text @click="deleteVisible = false" />
        <Button :label="lang.t('home.deleteConfirm')" severity="danger" @click="confirmDelete" />
      </template>
    </Dialog>
  </div>
</template>

<style scoped>
.page {
  height: 100%;
  overflow-y: auto;
  padding: 32px 40px 40px;
}

.page-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 24px;
  margin-bottom: 24px;
}

.page-head h1 {
  margin: 0 0 6px;
  font-size: 24px;
  font-weight: 600;
  letter-spacing: -0.01em;
}

.page-head p {
  font-size: 13px;
  color: var(--app-text-muted);
}

.identity {
  display: flex;
  align-items: stretch;
  justify-content: space-between;
  gap: 24px;
  padding: 24px 26px;
  border-radius: 16px;
  border: 1px solid var(--app-border);
  background:
    radial-gradient(120% 160% at 0% 0%, rgba(47, 124, 246, 0.16), transparent 58%),
    var(--app-surface);
}

.identity-main {
  min-width: 0;
}

.identity-label {
  font-size: 12px;
  color: var(--app-text-muted);
}

.identity-id {
  display: flex;
  align-items: center;
  gap: 10px;
  margin: 6px 0 8px;
}

.id-value {
  font-family: Consolas, "SFMono-Regular", monospace;
  font-size: 26px;
  font-weight: 600;
  letter-spacing: 0.08em;
  color: var(--app-text);
}

.copy-btn {
  width: 28px;
  height: 28px;
  display: grid;
  place-items: center;
  border: 1px solid var(--app-border);
  border-radius: 8px;
  color: var(--app-text-muted);
  background: rgba(255, 255, 255, 0.03);
  cursor: pointer;
}

.copy-btn:hover {
  color: var(--app-text);
  border-color: var(--app-border-strong);
}

.identity-name {
  font-size: 13px;
  color: var(--app-text-muted);
}

.copied {
  margin-left: 8px;
  color: var(--app-success);
}

.identity-status {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  min-width: 220px;
  padding-left: 24px;
  border-left: 1px solid var(--app-border);
}

.status-dot {
  width: 9px;
  height: 9px;
  margin-top: 5px;
  border-radius: 50%;
  background: var(--app-text-dim);
  box-shadow: 0 0 0 4px rgba(148, 163, 184, 0.12);
}

.status-dot.online {
  background: var(--app-success);
  box-shadow: 0 0 0 4px rgba(47, 191, 113, 0.16);
}

.status-title {
  font-size: 12px;
  color: var(--app-text-muted);
}

.status-value {
  margin: 2px 0 4px;
  font-size: 14px;
  font-weight: 500;
}

.status-url {
  font-size: 11px;
  font-family: Consolas, monospace;
  color: var(--app-text-dim);
  word-break: break-all;
}

.hint {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 14px 2px 28px;
  font-size: 12px;
  color: var(--app-text-dim);
}

.section-head {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 12px;
}

.section-head h2 {
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

.server-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.server-row {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 14px 16px;
  border-radius: 12px;
  border: 1px solid var(--app-border);
  background: var(--app-surface);
  transition: border-color 0.15s ease, background 0.15s ease;
}

.server-row:hover {
  border-color: var(--app-border-strong);
  background: var(--app-surface-hover);
}

.row-icon {
  width: 38px;
  height: 38px;
  flex: 0 0 38px;
  display: grid;
  place-items: center;
  border-radius: 10px;
  font-size: 15px;
  color: var(--app-accent);
  background: var(--app-icon-bg);
}

.row-main {
  min-width: 0;
  flex: 1;
}

.row-name {
  font-size: 14px;
  font-weight: 500;
}

.row-sub {
  margin-top: 2px;
  font-size: 12px;
  color: var(--app-text-muted);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.row-badge {
  padding: 3px 9px;
  border-radius: 999px;
  font-size: 11px;
  color: var(--app-text-dim);
  background: rgba(148, 163, 184, 0.1);
  white-space: nowrap;
}

.row-badge.online {
  color: #6fd39b;
  background: rgba(47, 191, 113, 0.12);
}

.row-actions {
  display: flex;
  align-items: center;
  gap: 2px;
}

.empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  text-align: center;
  padding: 54px 24px;
  border-radius: 16px;
  border: 1px dashed var(--app-border);
  background: var(--app-surface-soft);
}

.empty-icon {
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

.empty h3 {
  margin-bottom: 6px;
  font-size: 15px;
  font-weight: 600;
}

.empty p {
  margin-bottom: 18px;
  font-size: 13px;
  color: var(--app-text-muted);
}

.delete-message {
  font-size: 13px;
  line-height: 1.6;
  color: var(--app-text-muted);
}
</style>
