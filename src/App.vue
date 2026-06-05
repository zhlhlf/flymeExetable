<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { Quit, WindowGetPosition, WindowSetPosition, WindowSetSize } from '../wailsjs/runtime/runtime'
import { ChooseFolder, GetConfig, GetIcon, OpenItem, SaveWindowPosition, ScanFolders, SetFolders } from '../wailsjs/go/main/App'

type AppConfig = {
  folder: string | null
  folders?: string[]
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

const letters = ['#', ...'ABCDEFGHIJKLMNOPQRSTUVWXYZ'.split('')]
const folders = ref<string[]>([])
const settingsFolders = ref<string[]>([])
const items = ref<LauncherItem[]>([])
const activeLetter = ref('A')
const loading = ref(false)
const error = ref('')
const booting = ref(true)
const showSettings = ref(false)
const hoverOpenedPath = ref('')
const activeY = ref(Math.round(window.innerHeight / 2))
const expanded = ref(false)
const dockFocused = ref(false)
const choosingFolder = ref(false)
let collapseTimer = 0
let collapsedPosition: { x: number, y: number } | null = null
let expanding = false

const collapsedWidth = 58
const expandedWidth = 420
const expandedHeight = 820
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
const hasFolders = computed(() => folders.value.length > 0)

onMounted(async () => {
  try {
    const config = await GetConfig() as AppConfig
    currentWindowHeight = getCollapsedHeight()
    WindowSetSize(collapsedWidth, currentWindowHeight)
    if (config.windowPosition) {
      collapsedPosition = { x: config.windowPosition.x, y: config.windowPosition.y }
      WindowSetPosition(config.windowPosition.x, config.windowPosition.y)
    }
    folders.value = normalizeFolderList(config.folders?.length ? config.folders : (config.folder ? [config.folder] : []))
    settingsFolders.value = [...folders.value]
    if (folders.value.length > 0) {
      booting.value = false
      await refresh()
      return
    }
    showSettings.value = true
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
  if (collapseTimer) window.clearTimeout(collapseTimer)
})

async function chooseFolder() {
  cancelCollapse()
  showSettings.value = true
  await expandWindow()
  choosingFolder.value = true
  try {
    const selected = await ChooseFolder()
    await expandWindow()
    if (!selected) return

    const nextFolders = normalizeFolderList([...settingsFolders.value, selected])
    const config = await SetFolders(nextFolders)
    folders.value = normalizeFolderList(config.folders?.length ? config.folders : nextFolders)
    settingsFolders.value = [...folders.value]
    if (folders.value.length > 0) await refresh()
  } finally {
    choosingFolder.value = false
    await expandWindow()
    cancelCollapse()
  }
}

async function refresh() {
  if (!folders.value.length) return

  loading.value = true
  error.value = ''
  try {
    items.value = await ScanFolders(folders.value) as LauncherItem[]
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

function normalizeFolderList(source: string[]) {
  const seen = new Set<string>()
  const result: string[] = []
  for (const folder of source) {
    const value = folder.trim()
    if (!value) continue
    const key = value.toLowerCase()
    if (seen.has(key)) continue
    seen.add(key)
    result.push(value)
  }
  return result
}

async function openSettings() {
  await expandWindow()
  cancelCollapse()
  settingsFolders.value = [...folders.value]
  showSettings.value = true
}

function removeSettingsFolder(index: number) {
  settingsFolders.value = settingsFolders.value.filter((_, currentIndex) => currentIndex !== index)
}

async function confirmSettings() {
  const nextFolders = normalizeFolderList(settingsFolders.value)
  const config = await SetFolders(nextFolders)
  folders.value = normalizeFolderList(config.folders?.length ? config.folders : nextFolders)
  settingsFolders.value = [...folders.value]
  showSettings.value = false
  await refresh()
  scheduleCollapse()
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
  if (expanding) return
  expanding = true
  try {
    if (!expanded.value) {
      collapsedPosition = await WindowGetPosition()
    }
    const anchor = collapsedPosition ?? await WindowGetPosition()
    const collapsedHeight = getCollapsedHeight()
    const rightEdge = anchor.x + collapsedWidth
    const expandedX = rightEdge - width
    const expandedY = anchor.y - Math.round((expandedHeight - collapsedHeight) / 2)
    WindowSetSize(width, expandedHeight)
    WindowSetPosition(expandedX, expandedY)
    currentExpandedWidth = width
    currentWindowHeight = expandedHeight
    expanded.value = true
  } catch {
    // 忽略窗口扩展失败。
  } finally {
    expanding = false
  }
}

function cancelCollapse() {
  if (collapseTimer) {
    window.clearTimeout(collapseTimer)
    collapseTimer = 0
  }
}

function scheduleCollapse() {
  if (showSettings.value || choosingFolder.value) return
  dockFocused.value = false
  cancelCollapse()
  collapseTimer = window.setTimeout(async () => {
    if (!expanded.value) return
    try {
      const anchor = collapsedPosition ?? await WindowGetPosition()
      const collapsedHeight = getCollapsedHeight()
      const rightEdge = anchor.x + collapsedWidth
      WindowSetSize(collapsedWidth, collapsedHeight)
      WindowSetPosition(rightEdge - collapsedWidth, anchor.y)
      expanded.value = false
      currentExpandedWidth = collapsedWidth
      currentWindowHeight = collapsedHeight
      collapsedPosition = { x: anchor.x, y: anchor.y }
      await SaveWindowPosition(anchor.x, anchor.y)
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

function quitApp() {
  Quit()
}
</script>

<template>
  <main class="shell" :class="{ 'is-expanded': expanded }" @mouseenter="cancelCollapse" @mouseleave="scheduleCollapse">
    <section v-if="booting" class="empty glass-card booting">正在读取本地配置...</section>

    <section v-else-if="showSettings || !hasFolders" class="setup glass-card">
      <div class="setup-orb">A-Z</div>
      <h2>{{ hasFolders ? '启动器设置' : '首次运行需要指定文件夹' }}</h2>
      <p>可以添加多个目录，程序会合并扫描这些目录的当前层内容。</p>
      <div class="settings-list">
        <div v-if="!settingsFolders.length" class="settings-empty">还没有选择目录</div>
        <div v-for="(settingFolder, index) in settingsFolders" :key="settingFolder" class="settings-row">
          <span>{{ settingFolder }}</span>
          <button @click="removeSettingsFolder(index)">×</button>
        </div>
      </div>
      <div class="settings-actions">
        <button class="ghost big" @click="chooseFolder">添加目录</button>
        <button class="primary big" :disabled="!settingsFolders.length" @click="confirmSettings">确认</button>
      </div>
    </section>

    <template v-else>
      <aside class="alphabet-dock" aria-label="字母表" @mouseenter="cancelCollapse">
        <button class="settings-dot" title="设置" @click="openSettings">⚙</button>
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

      <div class="expanded-footer">
        <div class="footer-name">zhlhlf</div>
        <button class="footer-poem" title="退出应用" @click="quitApp">去年海棠玉殿惊 长袖当凤凰行</button>
      </div>
    </template>
  </main>
</template>
