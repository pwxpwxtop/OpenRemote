<script setup>
import { computed, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Button from 'primevue/button'
import Drawer from 'primevue/drawer'
import InputText from 'primevue/inputtext'
import Select from 'primevue/select'
import ToggleSwitch from 'primevue/toggleswitch'
import { useHubStore } from '../stores/hub'
import { useSettingStore } from '../stores/SettingConf'
import { useLanguageStore } from '../stores/Language'

const route = useRoute()
const router = useRouter()
const hub = useHubStore()
const settingStore = useSettingStore()
const lang = useLanguageStore()

const settingsVisible = ref(false)
const nameDraft = ref(hub.deviceName)

const localeModel = computed({
  get: () => lang.locale,
  set: (value) => lang.setLocale(value)
})

const navItems = computed(() => [
  { icon: 'fa-solid fa-desktop', label: lang.t('nav.devices'), path: '/' },
  { icon: 'fa-solid fa-server', label: lang.t('nav.addServer'), path: '/servers/add' },
  { icon: 'fa-solid fa-cloud', label: lang.t('nav.promotion'), path: '/promotion' }
])

function isActive(path) {
  return path === '/' ? route.path === '/' : route.path.startsWith(path)
}

function openSettings() {
  nameDraft.value = hub.deviceName
  settingsVisible.value = true
}

function saveName() {
  const next = nameDraft.value.trim()
  if (!next || next === hub.deviceName) return
  hub.saveDeviceName(next)
  if (hub.connected) {
    // 重新注册以便其他设备看到新名称
    hub.reconnect()
  }
}

function resetSettings() {
  settingStore.reset()
}
</script>

<template>
  <aside class="rail">
    <button type="button" class="rail-logo" :title="lang.t('app.name')" @click="router.push('/')">
      <i class="fa-solid fa-display"></i>
    </button>

    <nav class="rail-nav">
      <button
        v-for="item in navItems"
        :key="item.path"
        type="button"
        class="rail-item"
        :class="{ active: isActive(item.path) }"
        :title="item.label"
        @click="router.push(item.path)"
      >
        <i :class="item.icon"></i>
      </button>
    </nav>

    <div class="rail-spacer"></div>

    <button type="button" class="rail-item" :title="lang.t('nav.settings')" @click="openSettings">
      <i class="fa-solid fa-gear"></i>
    </button>

    <button type="button" class="rail-avatar" :title="hub.deviceName" @click="openSettings">
      <span>{{ hub.deviceName.slice(0, 1).toUpperCase() }}</span>
      <i class="rail-dot" :class="{ online: hub.connected }"></i>
    </button>
  </aside>

  <Drawer v-model:visible="settingsVisible" position="right" :style="{ width: '360px' }" :header="lang.t('settings.title')">
    <div class="settings-body">
      <section class="settings-section">
        <h3>{{ lang.t('settings.device') }}</h3>
        <p class="settings-hint">{{ lang.t('settings.deviceHint') }}</p>
        <label class="settings-label" for="device-name">{{ lang.t('settings.deviceName') }}</label>
        <InputText id="device-name" v-model="nameDraft" class="w-full" @blur="saveName" @keyup.enter="saveName" />
        <p class="settings-value">ID · {{ hub.localId.slice(0, 12) }}</p>
      </section>

      <section class="settings-section">
        <h3>{{ lang.t('settings.language') }}</h3>
        <p class="settings-hint">{{ lang.t('settings.languageHint') }}</p>
        <Select v-model="localeModel" :options="lang.locales" option-label="label" option-value="value" class="w-full" />
      </section>

      <section class="settings-section">
        <h3>{{ lang.t('settings.connection') }}</h3>
        <div class="settings-row">
          <div>
            <p class="settings-row-title">{{ lang.t('settings.fullscreenOnConnect') }}</p>
            <p class="settings-hint">{{ lang.t('settings.fullscreenOnConnectHint') }}</p>
          </div>
          <ToggleSwitch v-model="settingStore.settings.fullscreenOnConnect" />
        </div>
        <div class="settings-row">
          <div>
            <p class="settings-row-title">{{ lang.t('settings.confirmBeforeDisconnect') }}</p>
            <p class="settings-hint">{{ lang.t('settings.confirmBeforeDisconnectHint') }}</p>
          </div>
          <ToggleSwitch v-model="settingStore.settings.confirmBeforeDisconnect" />
        </div>
      </section>

      <Button :label="lang.t('settings.reset')" icon="pi pi-refresh" text severity="secondary" @click="resetSettings" />
    </div>
  </Drawer>
</template>

<style scoped>
.rail {
  width: var(--app-sidebar-width);
  flex: 0 0 var(--app-sidebar-width);
  height: 100%;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  padding: 12px 0 14px;
  background: var(--app-rail-bg);
  border-right: 1px solid var(--app-border);
}

.rail-logo {
  width: 40px;
  height: 40px;
  display: grid;
  place-items: center;
  border: none;
  border-radius: 12px;
  color: #fff;
  font-size: 17px;
  cursor: pointer;
  background: linear-gradient(135deg, #3d8bfd, var(--app-accent));
  box-shadow: 0 6px 16px rgba(47, 124, 246, 0.35);
}

.rail-nav {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin-top: 12px;
}

.rail-item {
  width: 42px;
  height: 42px;
  display: grid;
  place-items: center;
  border: none;
  border-radius: 11px;
  color: var(--app-text-muted);
  background: transparent;
  font-size: 17px;
  cursor: pointer;
  transition: background 0.15s ease, color 0.15s ease;
}

.rail-item:hover {
  color: var(--app-text);
  background: rgba(255, 255, 255, 0.06);
}

.rail-item.active {
  color: #8ab4ff;
  background: var(--app-accent-soft);
}

.rail-spacer {
  flex: 1;
}

.rail-avatar {
  position: relative;
  width: 36px;
  height: 36px;
  display: grid;
  place-items: center;
  border: 1px solid var(--app-border-strong);
  border-radius: 50%;
  font-size: 13px;
  font-weight: 600;
  color: var(--app-text);
  background: var(--app-surface);
  cursor: pointer;
}

.rail-dot {
  position: absolute;
  right: -1px;
  bottom: -1px;
  width: 10px;
  height: 10px;
  border-radius: 50%;
  border: 2px solid var(--app-rail-bg);
  background: var(--app-text-dim);
}

.rail-dot.online {
  background: var(--app-success);
}

.settings-body {
  display: flex;
  flex-direction: column;
  gap: 26px;
}

.settings-section h3 {
  margin: 0 0 4px;
  font-size: 14px;
  font-weight: 600;
}

.settings-hint {
  margin: 0 0 10px;
  font-size: 12px;
  line-height: 1.6;
  color: var(--app-text-muted);
}

.settings-label {
  display: block;
  margin-bottom: 6px;
  font-size: 12px;
  color: var(--app-text-muted);
}

.settings-value {
  margin: 8px 0 0;
  font-size: 11px;
  font-family: Consolas, monospace;
  color: var(--app-text-dim);
}

.settings-row {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  padding: 10px 0;
  border-top: 1px solid var(--app-border);
}

.settings-row-title {
  margin: 0 0 2px;
  font-size: 13px;
}

.w-full {
  width: 100%;
}
</style>
