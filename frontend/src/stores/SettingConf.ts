import { ref, watch } from 'vue'
import { defineStore } from 'pinia'

export interface SettingState {
  /** 连接后自动全屏 */
  fullscreenOnConnect: boolean
  /** 断开前确认 */
  confirmBeforeDisconnect: boolean
}

const STORAGE_KEY = 'openremote.settings'

const DEFAULT_SETTINGS: SettingState = {
  fullscreenOnConnect: false,
  confirmBeforeDisconnect: true
}

function loadSettings(): SettingState {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (raw) {
      const parsed = JSON.parse(raw) as Partial<SettingState>
      return {
        fullscreenOnConnect: Boolean(parsed.fullscreenOnConnect),
        confirmBeforeDisconnect: parsed.confirmBeforeDisconnect !== false
      }
    }
  } catch {
    // 配置损坏时回退到默认值
  }
  return { ...DEFAULT_SETTINGS }
}

export const useSettingStore = defineStore('setting', () => {
  const settings = ref<SettingState>(loadSettings())

  watch(settings, (value) => {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(value))
  }, { deep: true })

  function update(patch: Partial<SettingState>) {
    Object.assign(settings.value, patch)
  }

  function reset() {
    settings.value = { ...DEFAULT_SETTINGS }
  }

  return { settings, update, reset }
})
