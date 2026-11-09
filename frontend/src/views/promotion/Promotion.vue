<script setup>
import { computed } from 'vue'
import Button from 'primevue/button'
import { Browser } from '@wailsio/runtime'
import { useLanguageStore } from '../../stores/Language'
import tencentLogo from '../../../assets/promotion/tenxunyun.png'
import baiduLogo from '../../../assets/promotion/baiduyun.png'

const lang = useLanguageStore()

// 只展示腾讯云和百度云两个推荐入口。
const providers = computed(() => [
  {
    key: 'tencent',
    name: lang.t('promotion.tencentName'),
    desc: lang.t('promotion.tencentDesc'),
    logo: tencentLogo,
    accent: '#2f7cf6',
    url: 'https://curl.qcloud.com/8Bba0raB'
  },
  {
    key: 'baidu',
    name: lang.t('promotion.baiduName'),
    desc: lang.t('promotion.baiduDesc'),
    logo: baiduLogo,
    accent: '#4a6cf7',
    url: 'https://cloud.baidu.com/campaign/ambassador-product/index.html?ambassadorId=68d8bb6a19b443a3ae3af7b881f8bb8a'
  }
])

// 用系统默认浏览器打开推广链接；非 Wails 环境回退到新标签页。
async function openProvider(provider) {
  try {
    await Browser.OpenURL(provider.url)
  } catch {
    window.open(provider.url, '_blank', 'noopener')
  }
}
</script>

<template>
  <div class="page">
    <header class="page-head">
      <div>
        <h1>{{ lang.t('promotion.title') }}</h1>
        <p>{{ lang.t('promotion.subtitle') }}</p>
      </div>
    </header>

    <section class="provider-list">
      <article
        v-for="provider in providers"
        :key="provider.key"
        class="provider-card"
        :style="{ '--accent': provider.accent }"
        role="button"
        tabindex="0"
        @click="openProvider(provider)"
        @keydown.enter.prevent="openProvider(provider)"
        @keydown.space.prevent="openProvider(provider)"
      >
        <span class="provider-glow" aria-hidden="true"></span>

        <div class="provider-logo">
          <img :src="provider.logo" :alt="provider.name" />
        </div>

        <div class="provider-body">
          <h2>{{ provider.name }}</h2>
          <p>{{ provider.desc }}</p>
        </div>

        <Button
          class="provider-action"
          :label="lang.t('promotion.open')"
          icon="pi pi-arrow-right"
          icon-pos="right"
          text
          @click.stop="openProvider(provider)"
        />
      </article>
    </section>

    <p class="promo-hint">
      <i class="pi pi-info-circle"></i>
      {{ lang.t('promotion.hint') }}
    </p>
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
  margin-bottom: 26px;
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

.provider-list {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
  gap: 16px;
  max-width: 900px;
}

.provider-card {
  position: relative;
  display: flex;
  flex-direction: column;
  gap: 16px;
  padding: 24px;
  border-radius: 18px;
  border: 1px solid var(--app-border);
  background: var(--app-surface);
  overflow: hidden;
  cursor: pointer;
  outline: none;
  transition: transform 0.18s ease, border-color 0.18s ease, box-shadow 0.18s ease;
}

/* 顶部品牌色细线 */
.provider-card::before {
  content: '';
  position: absolute;
  top: 0;
  right: 0;
  left: 0;
  height: 2px;
  background: linear-gradient(90deg, var(--accent), transparent 72%);
  opacity: 0.6;
  transition: opacity 0.18s ease;
}

/* 右上角品牌色光晕 */
.provider-glow {
  position: absolute;
  top: -120px;
  right: -100px;
  width: 240px;
  height: 240px;
  border-radius: 50%;
  background: radial-gradient(circle, var(--accent), transparent 70%);
  opacity: 0.16;
  pointer-events: none;
  transition: opacity 0.18s ease;
}

.provider-card:hover,
.provider-card:focus-visible {
  border-color: var(--app-border-strong);
  transform: translateY(-2px);
  box-shadow: 0 14px 32px rgba(0, 0, 0, 0.3);
}

.provider-card:hover::before,
.provider-card:focus-visible::before {
  opacity: 1;
}

.provider-card:hover .provider-glow,
.provider-card:focus-visible .provider-glow {
  opacity: 0.28;
}

.provider-logo {
  position: relative;
  width: 58px;
  height: 58px;
  display: grid;
  place-items: center;
  border-radius: 16px;
  background: #fff;
  box-shadow: 0 2px 10px rgba(0, 0, 0, 0.18);
}

.provider-logo img {
  width: 34px;
  height: 34px;
  object-fit: contain;
  display: block;
}

.provider-body h2 {
  margin: 0;
  font-size: 16px;
  font-weight: 600;
  letter-spacing: -0.01em;
}

.provider-body p {
  margin: 8px 0 0;
  font-size: 12.5px;
  line-height: 1.65;
  color: var(--app-text-muted);
}

.provider-action {
  align-self: flex-start;
  margin-top: auto;
}

.provider-action.p-button {
  padding-left: 0;
}

.provider-action :deep(.p-button-icon) {
  transition: transform 0.18s ease;
}

.provider-card:hover .provider-action :deep(.p-button-icon) {
  transform: translateX(3px);
}

.promo-hint {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 22px;
  font-size: 12px;
  color: var(--app-text-dim);
}
</style>
