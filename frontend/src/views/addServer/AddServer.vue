<script setup>
import { computed, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import Button from 'primevue/button'
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import Password from 'primevue/password'
import Textarea from 'primevue/textarea'
import Toast from 'primevue/toast'
import { useToast } from 'primevue/usetoast'
import { HubService } from '../../../bindings/changeme'
import { useServerStore } from '../../stores/servers'
import { useHubStore } from '../../stores/hub'
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
const lang = useLanguageStore()
const toast = useToast()

const existing = serverStore.getById(props.id)
const editing = computed(() => Boolean(existing))

const form = reactive({
  name: existing?.name ?? '',
  host: existing?.host ?? '',
  port: existing?.port || 22,
  username: existing?.username ?? '',
  password: existing?.password ?? '',
  nodeUrl: existing?.nodeUrl ?? '',
  remark: existing?.remark ?? ''
})

const errors = reactive({
  name: '',
  host: '',
  port: ''
})

const deploying = ref(false)
const testing = ref(false)
const saving = ref(false)
const busy = computed(() => deploying.value || saving.value || testing.value)

function validate() {
  errors.name = form.name.trim() ? '' : lang.t('add.errorName')
  errors.host = form.host.trim() ? '' : lang.t('add.errorHost')
  const port = Number(form.port)
  errors.port = port >= 1 && port <= 65535 ? '' : lang.t('add.errorPort')
  return !errors.name && !errors.host && !errors.port
}

async function save() {
  if (!validate()) return

  // 不能重复添加同一台服务器
  const duplicate = serverStore.servers.find(
    (item) => item.id !== existing?.id && item.host.trim().toLowerCase() === form.host.trim().toLowerCase()
  )
  if (duplicate) {
    toast.add({
      severity: 'warn',
      summary: lang.t('add.duplicate'),
      life: 3000
    })
    return
  }

  saving.value = true
  try {
    // 保存前确保服务器上已装好转发节点：已安装直接加入，没有则自动通过 SSH 安装。
    if (!form.nodeUrl.trim()) {
      const nodeUrl = await ensureNode()
      if (!nodeUrl) return
    }

    const payload = {
      name: form.name.trim(),
      host: form.host.trim(),
      port: Number(form.port),
      username: form.username.trim(),
      password: form.password,
      nodeUrl: form.nodeUrl.trim(),
      remark: form.remark.trim()
    }

    if (existing) {
      serverStore.updateServer(existing.id, payload)
    } else {
      serverStore.addServer(payload)
    }

    // 保存后立即上线，这样列表里马上能看到该节点上的其他电脑。
    if (payload.nodeUrl) {
      await hub.connectTo(payload.nodeUrl)
    }

    toast.add({
      severity: 'success',
      summary: lang.t('add.saved'),
      life: 2500
    })
    router.push('/')
  } finally {
    saving.value = false
  }
}

// 检查服务器上的转发节点，没有就自动安装。
async function ensureNode() {
  deploying.value = true
  try {
    const nodeUrl = await HubService.EnsureNode(
      form.host.trim(),
      Number(form.port),
      form.username.trim() || 'root',
      form.password,
      9000
    )
    form.nodeUrl = nodeUrl
    toast.add({
      severity: 'success',
      summary: lang.t('add.nodeDeployed'),
      detail: nodeUrl,
      life: 3000
    })
    return nodeUrl
  } catch (err) {
    toast.add({
      severity: 'error',
      summary: lang.t('add.nodeFailed'),
      detail: err?.message ?? String(err),
      life: 5000
    })
    return ''
  } finally {
    deploying.value = false
  }
}

// 测试 SSH 连接；成功后自动在服务器上安装并启动转发功能。
async function testConnection() {
  if (!validate()) return
  if (!form.password.trim()) {
    toast.add({
      severity: 'warn',
      summary: lang.t('add.passwordRequired'),
      life: 3000
    })
    return
  }

  testing.value = true
  try {
    await HubService.TestConnection(
      form.host.trim(),
      Number(form.port),
      form.username.trim() || 'root',
      form.password
    )
    toast.add({
      severity: 'success',
      summary: lang.t('add.testOk'),
      life: 2500
    })
  } catch (err) {
    toast.add({
      severity: 'error',
      summary: lang.t('add.testFailed'),
      detail: err?.message ?? String(err),
      life: 5000
    })
    return
  } finally {
    testing.value = false
  }

  await ensureNode()
}

function cancel() {
  router.push('/')
}
</script>

<template>
  <div class="page">
    <Toast />

    <header class="page-header">
      <div>
        <h1>{{ editing ? lang.t('add.editTitle') : lang.t('add.title') }}</h1>
        <p>{{ lang.t('add.subtitle') }}</p>
      </div>
    </header>

    <form class="form-card" @submit.prevent="save">
      <div class="field field--full">
        <label class="field-label" for="server-name">{{ lang.t('add.name') }}</label>
        <InputText
          id="server-name"
          v-model="form.name"
          fluid
          :placeholder="lang.t('add.namePlaceholder')"
        />
        <small v-if="errors.name" class="field-error">{{ errors.name }}</small>
      </div>

      <div class="field-row">
        <div class="field field--grow">
          <label class="field-label" for="server-host">{{ lang.t('add.host') }}</label>
          <InputText
            id="server-host"
            v-model="form.host"
            fluid
            :placeholder="lang.t('add.hostPlaceholder')"
          />
          <small v-if="errors.host" class="field-error">{{ errors.host }}</small>
        </div>
        <div class="field field--port">
          <label class="field-label" for="server-port">{{ lang.t('add.port') }}</label>
          <InputNumber
            id="server-port"
            v-model="form.port"
            fluid
            :min="1"
            :max="65535"
            :use-grouping="false"
          />
          <small v-if="errors.port" class="field-error">{{ errors.port }}</small>
        </div>
      </div>

      <div class="field-row">
        <div class="field">
          <label class="field-label">{{ lang.t('add.username') }}</label>
          <InputText
            v-model="form.username"
            fluid
            :placeholder="lang.t('add.usernamePlaceholder')"
          />
        </div>
        <div class="field">
          <label class="field-label">{{ lang.t('add.password') }}</label>
          <Password
            v-model="form.password"
            fluid
            toggle-mask
            :feedback="false"
            :placeholder="lang.t('add.passwordPlaceholder')"
          />
        </div>
      </div>
      <small class="field-hint">{{ lang.t('add.sshHint') }}</small>

      <div class="field field--full">
        <label class="field-label">{{ lang.t('add.remark') }}</label>
        <Textarea
          v-model="form.remark"
          rows="3"
          auto-resize
          fluid
          :placeholder="lang.t('add.remarkPlaceholder')"
        />
      </div>

      <footer class="form-actions">
        <Button
          type="button"
          :label="lang.t('add.test')"
          icon="pi pi-bolt"
          severity="secondary"
          outlined
          :loading="testing"
          :disabled="busy"
          class="test-button"
          @click="testConnection"
        />
        <Button
          type="button"
          :label="lang.t('add.cancel')"
          severity="secondary"
          text
          @click="cancel"
        />
        <Button
          type="submit"
          :label="lang.t('add.save')"
          icon="pi pi-check"
          :loading="busy"
          :disabled="busy"
        />
      </footer>
    </form>
  </div>
</template>

<style scoped>
.page {
  height: 100%;
  overflow-y: auto;
  padding: 32px 40px 40px;
}

.page-header {
  margin-bottom: 24px;
}

.page-header h1 {
  margin: 0 0 6px;
  font-size: 26px;
  font-weight: 600;
  letter-spacing: -0.01em;
}

.page-header p {
  margin: 0;
  font-size: 13px;
  color: var(--app-text-muted);
}

.form-card {
  max-width: 720px;
  display: flex;
  flex-direction: column;
  gap: 18px;
  padding: 26px;
  border-radius: 16px;
  background: var(--app-surface);
  border: 1px solid var(--app-border);
}

.field {
  display: flex;
  flex-direction: column;
  gap: 7px;
  min-width: 0;
}

.field-row {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 18px;
  align-items: start;
}

.field-row .field--port {
  grid-column: 2;
}

.field-row:has(.field--grow) {
  grid-template-columns: minmax(0, 1fr) 140px;
}

.field-label {
  font-size: 12px;
  font-weight: 500;
  color: var(--app-text-muted);
}

.field-error {
  font-size: 12px;
  color: #f87171;
}

.field-hint {
  font-size: 12px;
  line-height: 1.5;
  color: var(--app-text-muted);
}

.form-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 6px;
  padding-top: 18px;
  border-top: 1px solid var(--app-border);
}

.test-button {
  margin-right: auto;
}
</style>
