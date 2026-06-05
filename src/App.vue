<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { WindowGetPosition, WindowSetPosition, WindowSetSize } from '../wailsjs/runtime/runtime'
import { ChooseFolder, GetConfig, GetIcon, OpenItem, SaveWindowPosition, ScanFolder } from '../wailsjs/go/main/App'

type AppConfig = {
  folder: string | null
  windowPosition?: {
    x: number
    y: number
  } | null
}

type LauncherItem = {
  name: string
  path: string
  letter: string
  extension: string
  isDir: boolean
  icon: string
}

const letters = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ'.split('')
const folder = ref<string>('')
const items = ref<LauncherItem[]>([])
const activeLetter = ref('A')
const loading = ref(false)
const error = ref('')
const booting = ref(true)
const hoverOpenedPath = ref('')
const activeY = ref(Math.round(window.innerHeight / 2))
const expanded = ref(false)
const dockFocused = ref(false)
let savePositionTimer = 0
let collapseTimer = 0

const collapsedWidth = 58
const expandedWidth = 420
const expandedHeight = 680
let currentExpandedWidth = collapsedWidth
let currentWindowHeight = 86

const grouped = computed(() => {
  const result = new Map<string, LauncherItem[]>()
  for (const letter of letters) result.set(letter, [])
  result.set('#', [])

  for (const item of items.value) {
    const key = letters.includes(item.letter) ? item.letter : '#'
    result.get(key)?.push(item)
  }

  return result
})

const activeItems = computed(() => grouped.value.get(activeLetter.value) ?? [])
const totalItems = computed(() => items.value.length)
const visibleLetters = computed(() => letters.filter((letter) => (grouped.value.get(letter)?.length ?? 0) > 0))

onMounted(async () => {
  try {
    const config = await GetConfig() as AppConfig
    currentWindowHeight = getCollapsedHeight()
    WindowSetSize(collapsedWidth, currentWindowHeight)
    if (config.windowPosition) {
      WindowSetPosition(config.windowPosition.x, config.windowPosition.y)
    }
    startPositionRecorder()
    if (config.folder) {
      folder.value = config.folder
      booting.value = false
      await refresh()
      return
    }
    currentExpandedWidth = expandedWidth
    currentWindowHeight = expandedHeight
    WindowSetSize(currentExpandedWidth, currentWindowHeight)
    expanded.value = true
  } catch (err) {
    error.value = String(err)
  } finally {
    booting.value = false
  }
})

onBeforeUnmount(() => {
  if (savePositionTimer) window.clearInterval(savePositionTimer)
  if (collapseTimer) window.clearTimeout(collapseTimer)
})

function startPositionRecorder() {
  if (savePositionTimer) return
  let last = ''
  savePositionTimer = window.setInterval(async () => {
    try {
      if (expanded.value) return
      const pos = await WindowGetPosition()
      const key = `${pos.x},${pos.y}`
      if (key === last) return
      last = key
      await SaveWindowPosition(pos.x, pos.y)
    } catch {
      // 忽略位置保存失败，避免影响启动器使用。
    }
  }, 1200)
}

async function chooseFolder() {
  await expandWindow()
  const selected = await ChooseFolder()
  if (!selected) return

  folder.value = selected
  await refresh()
  scheduleCollapse()
}

async function refresh() {
  if (!folder.value) return

  loading.value = true
  error.value = ''
  try {
    items.value = await ScanFolder(folder.value) as LauncherItem[]
    const firstAvailable = visibleLetters.value[0]
    activeLetter.value = firstAvailable ?? 'A'
    if (!expanded.value) {
      currentWindowHeight = getCollapsedHeight()
      WindowSetSize(collapsedWidth, currentWindowHeight)
    }
    void loadIcons(items.value)
  } catch (err) {
    error.value = String(err)
  } finally {
    loading.value = false
  }
}

async function loadIcons(sourceItems: LauncherItem[]) {
  for (const item of sourceItems) {
    if (item.icon) continue
    try {
      const icon = await GetIcon(item.path)
      if (!icon) continue
      const current = items.value.find((candidate) => candidate.path === item.path)
      if (current) current.icon = icon
    } catch {
      // 单个文件图标提取失败时忽略，继续展示默认图标。
    }
  }
}

async function openItem(item: LauncherItem) {
  await OpenItem(item.path)
}

async function openItemOnHover(item: LauncherItem) {
  if (hoverOpenedPath.value === item.path) return
  hoverOpenedPath.value = item.path
  await openItem(item)
  window.setTimeout(() => {
    if (hoverOpenedPath.value === item.path) hoverOpenedPath.value = ''
  }, 900)
}

function onLetterEnter(letter: string, event: MouseEvent) {
  cancelCollapse()
  dockFocused.value = true
  activeLetter.value = letter
  const target = event.currentTarget as HTMLElement
  const rect = target.getBoundingClientRect()
  activeY.value = Math.round(rect.top + rect.height / 2)
  void expandWindow()
}

function getCollapsedHeight() {
  const lettersHeight = Math.max(1, visibleLetters.value.length) * 20
  return Math.min(Math.max(lettersHeight + 42, 78), expandedHeight)
}

async function expandWindow(width = expandedWidth) {
  if (collapseTimer) window.clearTimeout(collapseTimer)
  try {
    const pos = await WindowGetPosition()
    const fromWidth = expanded.value ? currentExpandedWidth : collapsedWidth
    const fromHeight = expanded.value ? currentWindowHeight : getCollapsedHeight()
    WindowSetPosition(pos.x - (width - fromWidth), pos.y - Math.round((expandedHeight - fromHeight) / 2))
    WindowSetSize(width, expandedHeight)
    currentExpandedWidth = width
    currentWindowHeight = expandedHeight
    expanded.value = true
  } catch {
    // 忽略窗口扩展失败。
  }
}

function cancelCollapse() {
  if (collapseTimer) {
    window.clearTimeout(collapseTimer)
    collapseTimer = 0
  }
}

function scheduleCollapse() {
  dockFocused.value = false
  cancelCollapse()
  collapseTimer = window.setTimeout(async () => {
    if (!expanded.value) return
    try {
      const pos = await WindowGetPosition()
      const collapsedHeight = getCollapsedHeight()
      const collapsedX = pos.x + (currentExpandedWidth - collapsedWidth)
      const collapsedY = pos.y + Math.round((currentWindowHeight - collapsedHeight) / 2)
      WindowSetSize(collapsedWidth, collapsedHeight)
      WindowSetPosition(collapsedX, collapsedY)
      expanded.value = false
      currentExpandedWidth = collapsedWidth
      currentWindowHeight = collapsedHeight
      await SaveWindowPosition(collapsedX, collapsedY)
    } catch {
      expanded.value = false
      currentExpandedWidth = collapsedWidth
      currentWindowHeight = getCollapsedHeight()
    }
  }, 520)
}

function iconOf(item: LauncherItem) {
  if (item.isDir) return '📁'
  const ext = item.extension.toLowerCase()
  if (ext === 'exe') return '⚡'
  if (ext === 'lnk') return '↗'
  if (ext === 'url') return '🌐'
  if (['bat', 'cmd', 'ps1'].includes(ext)) return '⌘'
  return '✨'
}
</script>

<template>
  <main class="shell" :class="{ 'is-expanded': expanded }" @mouseenter="cancelCollapse" @mouseleave="scheduleCollapse">
    <section v-if="booting" class="empty glass-card booting">正在读取本地配置...</section>

    <section v-else-if="!folder" class="setup glass-card">
      <div class="setup-orb">A-Z</div>
      <h2>首次运行需要指定文件夹</h2>
      <p>后续会直接读取 <code>~/.jczhl-filyme</code> 中保存的配置，不再弹出选择。</p>
      <button class="primary big" @click="chooseFolder">选择文件夹并开始</button>
    </section>

    <template v-else>
      <aside class="alphabet-dock" aria-label="字母表" @mouseenter="cancelCollapse">
        <button class="refresh-dot" :disabled="loading" title="刷新" @click="refresh">↻</button>
        <button
          v-for="letter in visibleLetters"
          :key="letter"
          class="letter"
          :class="{ active: dockFocused && activeLetter === letter, filled: (grouped.get(letter)?.length ?? 0) > 0 }"
          @mouseenter="onLetterEnter(letter, $event)"
          @click="activeLetter = letter"
        >
          {{ letter }}
        </button>
      </aside>

      <div class="fly-panel glass-card" :style="{ top: `${activeY}px` }" @mouseenter="cancelCollapse">
        <div class="fly-head">
          <b>{{ activeLetter }}</b>
          <span>{{ loading ? '刷新中' : `${activeItems.length}/${totalItems}` }}</span>
        </div>
        <div v-if="error" class="fly-empty">{{ error }}</div>
        <div v-else-if="!activeItems.length" class="fly-empty">暂无</div>
        <template v-else>
          <button v-for="item in activeItems" :key="`fly-${item.path}`" class="fly-item" @click="openItem(item)">
            <span class="fly-icon">
              <img v-if="item.icon" :src="item.icon" :alt="item.name" />
              <span v-else>{{ iconOf(item) }}</span>
            </span>
            <strong>{{ item.name }}</strong>
          </button>
        </template>
      </div>
    </template>
  </main>
</template>
