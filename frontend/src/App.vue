<script setup>
import { computed, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import AppSidebar from './components/AppSidebar.vue'
import { HubService } from '../bindings/changeme'
import { useHubStore } from './stores/hub'
import { useServerStore } from './stores/servers'

const route = useRoute()
const hub = useHubStore()
const serverStore = useServerStore()

const standalone = computed(() => route.meta.standalone === true)

async function bootstrap() {
  if (hub.connected) return

  const server = serverStore.lastConnectedServer ?? serverStore.servers[0]
  if (!server) return

  let url = server.nodeUrl || ''

  // 启动时确保服务器上的中转节点还在运行；没有安装则自动安装。
  if (server.password) {
    try {
      const ensured = await HubService.EnsureNode(
        server.host,
        server.port,
        server.username || 'root',
        server.password,
        9000
      )
      if (ensured) url = ensured
    } catch {
      // 自动部署失败时仍尝试用已保存的地址连接
    }
  }

  if (url) {
    if (server.nodeUrl !== url) {
      serverStore.updateServer(server.id, { nodeUrl: url })
    }
    await hub.connectTo(url)
  }
}

onMounted(bootstrap)
</script>

<template>
  <div class="app-shell">
    <AppSidebar v-if="!standalone" />
    <main class="app-main">
      <router-view />
    </main>
  </div>
</template>

<style scoped>
.app-shell {
  display: flex;
  height: 100vh;
  overflow: hidden;
  background: var(--app-bg);
}

.app-main {
  flex: 1;
  min-width: 0;
  height: 100%;
  overflow: hidden;
}
</style>
