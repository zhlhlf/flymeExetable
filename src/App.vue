<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { Quit, ScreenGetAll, WindowGetPosition, WindowSetPosition, WindowSetSize } from '../wailsjs/runtime/runtime'
import { ChooseFolder, GetCachedItems, GetConfig, GetIcons, GetTypeIcons, OpenItem, OpenItemLocation, SaveWindowPosition, ScanFolders, SetSettings } from '../wailsjs/go/main/App'
import appIcon from '../appicon-rounded.png'

type AppConfig = {
  folder: string | null
  folders?: string[]
  windowPosition?: {
    x: number
    y: number
  } | null
  poem?: string
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
const letterSet = new Set(letters)
const defaultPoem = '鍘诲勾娴锋鐜夋鎯?闀胯褰撳嚖鍑拌'
const folders = ref<string[]>([])
const settingsFolders = ref<string[]>([])
const poem = ref(defaultPoem)
const settingsPoem = ref(defaultPoem)
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
let savePositionTimer = 0
let collapsedPosition: { x: number, y: number } | null = null
let expanding = false
let collapseToken = 0
let savingPosition = false
let pendingPosition: { x: number, y: number } | null = null
let lastSavedPosition = ''

const collapsedWidth = 58
const expandedWidth = 360
const setupWidth = 392
const setupHeight = 500
const minExpandedHeight = 520
const maxExpandedHeight = 920
const flyPanelHeight = 318
const dockTop = 8
const dockControlsHeight = 64
const letterHeight = 20
const flyPanelOffsetY = 26
const flyPanelBottomGap = 18
let currentExpandedWidth = collapsedWidth
let currentWindowHeight = 86
const typeIconPromises = new Map<string, Promise<string>>()
const itemIconPromises = new Map<string, Promise<string>>()
const fallbackIconUrls = new Map<string, string>()
let iconLoadToken = 0
const iconConcurrency = 8

type GradientTheme = {
  shell: string
  card: string
}

function randomHue() {
  return Math.floor(Math.random() * 360)
}

function hueShift(hue: number, shift: number) {
  return (hue + shift + 360) % 360
}

function hsl(hue: number, saturation: number, lightness: number, alpha: number) {
  return `hsla(${hue}, ${saturation}%, ${lightness}%, ${alpha})`
}

function randomGradientTheme(): GradientTheme {
  const primary = randomHue()
  const secondary = hueShift(primary, 80 + Math.floor(Math.random() * 120))
  const accent = hueShift(primary, 190 + Math.floor(Math.random() * 110))
  const baseA = hueShift(primary, -8 + Math.floor(Math.random() * 16))
  const baseB = hueShift(secondary, -12 + Math.floor(Math.random() * 24))

  return {
    shell: [
      `radial-gradient(circle at ${72 + Math.floor(Math.random() * 12)}% ${38 + Math.floor(Math.random() * 14)}%, ${hsl(primary, 92, 64, .24)}, transparent 34%)`,
      `radial-gradient(circle at ${16 + Math.floor(Math.random() * 16)}% ${12 + Math.floor(Math.random() * 16)}%, ${hsl(secondary, 88, 66, .20)}, transparent 36%)`,
      `radial-gradient(circle at ${32 + Math.floor(Math.random() * 28)}% ${74 + Math.floor(Math.random() * 14)}%, ${hsl(accent, 86, 64, .14)}, transparent 38%)`,
      `linear-gradient(135deg, ${hsl(baseA, 64, 17, .34)}, ${hsl(baseB, 62, 15, .22)})`,
    ].join(', '),
    card: `linear-gradient(135deg, ${hsl(primary, 90, 66, .15)}, ${hsl(secondary, 88, 68, .10)})`,
  }
}

const gradientTheme = ref(randomGradientTheme())
const shellStyle = computed(() => {
  const theme = gradientTheme.value
  return {
    '--shell-bg': theme.shell,
    '--card-bg': theme.card,
  }
})

function changeGradientTheme() {
  gradientTheme.value = randomGradientTheme()
}

const grouped = computed(() => {
  const result = new Map<string, LauncherItem[]>()
  for (const letter of letters) result.set(letter, [])

  for (const item of items.value) {
    const key = letterSet.has(item.letter) ? item.letter : '#'
    result.get(key)?.push(item)
  }

  return result
})

const activeItems = computed(() => grouped.value.get(activeLetter.value) ?? [])
const totalItems = computed(() => items.value.length)
const visibleLetters = computed(() => letters.filter((letter) => (grouped.value.get(letter)?.length ?? 0) > 0))
const hasFolders = computed(() => folders.value.length > 0)
const sidePoemLines = computed(() => {
  const parts = poem.value.trim().split(/\s+/).filter(Boolean)
  if (parts.length >= 2) return [parts[0], parts.slice(1).join('')]
  return [poem.value.trim() || defaultPoem, '']
})

onMounted(async () => {
  try {
    const config = await GetConfig() as AppConfig
    const initialPosition = config.windowPosition ?? await WindowGetPosition()
    collapsedPosition = { x: initialPosition.x, y: initialPosition.y }
    lastSavedPosition = config.windowPosition ? positionKey(collapsedPosition) : ''
    poem.value = config.poem?.trim() || defaultPoem
    folders.value = normalizeFolderList(config.folders?.length ? config.folders : (config.folder ? [config.folder] : []))
    settingsFolders.value = [...folders.value]
    settingsPoem.value = poem.value
    if (folders.value.length > 0) {
      currentWindowHeight = getCollapsedHeight()
      WindowSetSize(collapsedWidth, currentWindowHeight)
      WindowSetPosition(collapsedPosition.x, collapsedPosition.y)
      startPositionRecorder()
      booting.value = false
      await bootstrapItems()
      return
    }
    showSettings.value = true
    await openSetupWindow(true)
    startPositionRecorder()
  } catch (err) {
    error.value = String(err)
  } finally {
    booting.value = false
  }
})

onBeforeUnmount(() => {
  if (collapseTimer) window.clearTimeout(collapseTimer)
  if (savePositionTimer) window.clearInterval(savePositionTimer)
})

function positionKey(position: { x: number, y: number }) {
  return `${position.x},${position.y}`
}

async function saveCollapsedPosition(position: { x: number, y: number }) {
  const key = positionKey(position)
  collapsedPosition = { x: position.x, y: position.y }
  if (key === lastSavedPosition) return
  pendingPosition = { x: position.x, y: position.y }
  if (savingPosition) return

  savingPosition = true
  try {
    while (pendingPosition) {
      const next = pendingPosition
      pendingPosition = null
      const nextKey = positionKey(next)
      if (nextKey === lastSavedPosition) continue
      try {
        await SaveWindowPosition(next.x, next.y)
        lastSavedPosition = nextKey
      } catch {
        pendingPosition = next
        return
      }
    }
  } finally {
    savingPosition = false
  }
}

function startPositionRecorder() {
  if (savePositionTimer) return
  savePositionTimer = window.setInterval(async () => {
    if (expanded.value || showSettings.value || choosingFolder.value) return
    try {
      const position = await WindowGetPosition()
      if (expanded.value || showSettings.value || choosingFolder.value) return
      await saveCollapsedPosition(position)
    } catch {
      // Ignore position polling failures.
    }
  }, 1000)
}

async function chooseFolder() {
  cancelCollapse()
  showSettings.value = true
  choosingFolder.value = true
  try {
    const selected = await ChooseFolder()
    if (!selected) return

    const nextFolders = normalizeFolderList([...settingsFolders.value, selected])
    const config = await SetSettings(nextFolders, settingsPoem.value)
    folders.value = normalizeFolderList(config.folders?.length ? config.folders : nextFolders)
    settingsFolders.value = [...folders.value]
    poem.value = config.poem?.trim() || defaultPoem
    settingsPoem.value = poem.value
    if (folders.value.length > 0) await refresh()
  } finally {
    choosingFolder.value = false
    cancelCollapse()
  }
}

function applyItems(nextItems: LauncherItem[], preferKeepLetter = true) {
  const previousLetter = activeLetter.value
  items.value = nextItems
  const available = visibleLetters.value
  if (preferKeepLetter && available.includes(previousLetter)) {
    activeLetter.value = previousLetter
  } else {
    activeLetter.value = available[0] ?? 'A'
  }
  if (!expanded.value) {
    currentWindowHeight = getCollapsedHeight()
    WindowSetSize(collapsedWidth, currentWindowHeight)
  }
  void loadActiveIcons()
}

async function bootstrapItems() {
  if (!folders.value.length) return

  error.value = ''
  let usedCache = false
  try {
    const cached = await GetCachedItems(folders.value) as LauncherItem[]
    if (cached?.length) {
      applyItems(cached, false)
      usedCache = true
    }
  } catch {
    // ignore cache read failures and continue with live scan
  }

  if (!usedCache) loading.value = true
  try {
    const fresh = await ScanFolders(folders.value) as LauncherItem[]
    if (!usedCache || itemsFingerprint(fresh) !== itemsFingerprint(items.value)) {
      applyItems(fresh, usedCache)
    } else {
      void loadActiveIcons()
    }
  } catch (err) {
    if (!usedCache) error.value = String(err)
  } finally {
    loading.value = false
  }
}

function itemsFingerprint(list: LauncherItem[]) {
  return list.map((item) => item.path + '|' + item.name + '|' + item.letter + '|' + (item.isDir ? '1' : '0') + '|' + item.extension).join('\n')
}

async function refresh() {
  if (!folders.value.length) return

  loading.value = true
  error.value = ''
  try {
    const fresh = await ScanFolders(folders.value) as LauncherItem[]
    applyItems(fresh, true)
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
  await openSetupWindow(true)
  cancelCollapse()
  settingsFolders.value = [...folders.value]
  settingsPoem.value = poem.value
  showSettings.value = true
}

function removeSettingsFolder(index: number) {
  settingsFolders.value = settingsFolders.value.filter((_, currentIndex) => currentIndex !== index)
}

async function confirmSettings() {
  const nextFolders = normalizeFolderList(settingsFolders.value)
  const config = await SetSettings(nextFolders, settingsPoem.value)
  folders.value = normalizeFolderList(config.folders?.length ? config.folders : nextFolders)
  settingsFolders.value = [...folders.value]
  poem.value = config.poem?.trim() || defaultPoem
  settingsPoem.value = poem.value
  showSettings.value = false
  await refresh()
  scheduleCollapse()
}

watch(activeItems, () => {
  void loadActiveIcons()
})

function setItemIcon(path: string, icon: string) {
  if (!icon) return
  const index = items.value.findIndex((candidate) => candidate.path === path)
  if (index < 0) return
  if (items.value[index].icon === icon) return
  const next = items.value.slice()
  next[index] = { ...next[index], icon }
  items.value = next
}

async function loadActiveIcons() {
  const token = ++iconLoadToken
  const sourceItems = activeItems.value.filter((item) => !item.icon)
  if (!sourceItems.length) return

  const uniquePaths: string[] = []
  const pathSeen = new Set<string>()
  const typeKeys: string[] = []
  const typeSeen = new Set<string>()
  const typeOwners = new Map<string, string[]>()

  for (const item of sourceItems) {
    if (item.isDir) {
      if (!typeSeen.has('folder')) {
        typeSeen.add('folder')
        typeKeys.push('folder')
      }
      const list = typeOwners.get('folder') ?? []
      list.push(item.path)
      typeOwners.set('folder', list)
      continue
    }

    const ext = item.extension.toLowerCase()
    if (ext === 'lnk' || ext === 'exe') {
      if (!pathSeen.has(item.path)) {
        pathSeen.add(item.path)
        uniquePaths.push(item.path)
      }
      continue
    }

    if (!ext) continue
    if (!typeSeen.has(ext)) {
      typeSeen.add(ext)
      typeKeys.push(ext)
    }
    const list = typeOwners.get(ext) ?? []
    list.push(item.path)
    typeOwners.set(ext, list)
  }

  const tasks: Array<() => Promise<void>> = []

  if (uniquePaths.length) {
    tasks.push(async () => {
      try {
        const icons = await GetIcons(uniquePaths) as Record<string, string>
        if (token !== iconLoadToken) return
        for (const [path, icon] of Object.entries(icons || {})) {
          if (!icon) continue
          itemIconPromises.set(path, Promise.resolve(icon))
          setItemIcon(path, icon)
        }
      } catch {
        // ignore batch failures; fallback icons remain
      }
    })
  }

  if (typeKeys.length) {
    tasks.push(async () => {
      try {
        const icons = await GetTypeIcons(typeKeys) as Record<string, string>
        if (token !== iconLoadToken) return
        for (const [ext, icon] of Object.entries(icons || {})) {
          if (!icon) continue
          typeIconPromises.set(ext, Promise.resolve(icon))
          const owners = typeOwners.get(ext) ?? []
          for (const path of owners) setItemIcon(path, icon)
        }
      } catch {
        // ignore batch failures; fallback icons remain
      }
    })
  }

  let cursor = 0
  async function worker() {
    while (cursor < tasks.length) {
      const current = cursor++
      await tasks[current]()
    }
  }
  const workers = Array.from({ length: Math.min(iconConcurrency, Math.max(tasks.length, 1)) }, () => worker())
  await Promise.all(workers)
}

async function openItem(item: LauncherItem) {
  await OpenItem(item.path)
}

async function revealItem(item: LauncherItem, event?: MouseEvent) {
  event?.preventDefault()
  event?.stopPropagation()
  cancelCollapse()
  try {
    await OpenItemLocation(item.path)
  } catch (err) {
    error.value = String(err)
  }
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
  return Math.min(Math.max(lettersHeight + 42, 78), getExpandedHeight())
}

function getExpandedHeight() {
  const lettersCount = Math.max(1, visibleLetters.value.length)
  const lastLetterCenter = dockTop + dockControlsHeight + ((lettersCount - 1) * letterHeight) + (letterHeight / 2)
  const heightForLastBubble = Math.ceil(lastLetterCenter - flyPanelOffsetY + flyPanelHeight + flyPanelBottomGap)
  return Math.min(Math.max(heightForLastBubble, minExpandedHeight), maxExpandedHeight)
}

async function getCurrentScreenSize() {
  try {
    const screens = await ScreenGetAll()
    const screen = screens.find((item) => item.isCurrent) ?? screens.find((item) => item.isPrimary) ?? screens[0]
    if (screen?.width && screen?.height) return { width: screen.width, height: screen.height }
  } catch {
    // Ignore screen lookup failures.
  }
  return { width: window.screen.availWidth || 1280, height: window.screen.availHeight || 720 }
}

async function openSetupWindow(center = false) {
  clearCollapseTimer()
  collapseToken++
  const width = setupWidth
  const height = setupHeight
  const screen = await getCurrentScreenSize()
  const fallbackX = Math.max(16, Math.round((screen.width - width) / 2))
  const fallbackY = Math.max(16, Math.round((screen.height - height) / 2))
  const anchor = center ? { x: fallbackX, y: fallbackY } : (collapsedPosition ?? await WindowGetPosition())
  const x = Math.min(Math.max(anchor.x, 12), Math.max(12, screen.width - width - 12))
  const y = Math.min(Math.max(anchor.y, 12), Math.max(12, screen.height - height - 12))
  WindowSetSize(width, height)
  WindowSetPosition(x, y)
  currentExpandedWidth = width
  currentWindowHeight = height
  expanded.value = true
}

function clearCollapseTimer() {
  if (collapseTimer) {
    window.clearTimeout(collapseTimer)
    collapseTimer = 0
  }
}

async function expandWindow(width = expandedWidth) {
  clearCollapseTimer()
  collapseToken++
  if (expanding) return

  expanding = true
  try {
    const wasCollapsed = !expanded.value
    if (wasCollapsed) {
      const position = await WindowGetPosition()
      collapsedPosition = { x: position.x, y: position.y }
    }

    const anchor = collapsedPosition ?? await WindowGetPosition()

    const collapsedHeight = getCollapsedHeight()
    const expandedHeight = getExpandedHeight()
    const rightEdge = anchor.x + collapsedWidth
    const expandedX = rightEdge - width
    const expandedY = anchor.y - Math.round((expandedHeight - collapsedHeight) / 2)
    WindowSetSize(width, expandedHeight)
    WindowSetPosition(expandedX, expandedY)
    currentExpandedWidth = width
    currentWindowHeight = expandedHeight
    expanded.value = true
    if (wasCollapsed) changeGradientTheme()
  } catch {
    // Ignore window expansion failures.
  } finally {
    expanding = false
  }
}

function cancelCollapse() {
  collapseToken++
  clearCollapseTimer()
}

function scheduleCollapse() {
  if (showSettings.value || choosingFolder.value) return
  dockFocused.value = false
  clearCollapseTimer()
  const token = ++collapseToken
  collapseTimer = window.setTimeout(async () => {
    collapseTimer = 0
    if (token !== collapseToken || !expanded.value) return
    try {
      const anchor = collapsedPosition ?? await WindowGetPosition()
      if (token !== collapseToken || !expanded.value) return
      const collapsedHeight = getCollapsedHeight()
      const rightEdge = anchor.x + collapsedWidth
      WindowSetSize(collapsedWidth, collapsedHeight)
      WindowSetPosition(rightEdge - collapsedWidth, anchor.y)
      expanded.value = false
      currentExpandedWidth = collapsedWidth
      currentWindowHeight = collapsedHeight
      collapsedPosition = { x: anchor.x, y: anchor.y }
      changeGradientTheme()
      await saveCollapsedPosition(anchor)
    } catch {
      if (token !== collapseToken) return
      expanded.value = false
      currentExpandedWidth = collapsedWidth
      currentWindowHeight = getCollapsedHeight()
      changeGradientTheme()
    }
  }, 520)
}

function fileIconUrl(kind: string, label: string, color: string) {
  const cacheKey = `${kind}:${label}:${color}`
  const cached = fallbackIconUrls.get(cacheKey)
  if (cached) return cached
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="64" height="64" viewBox="0 0 64 64"><defs><linearGradient id="g" x1="12" y1="6" x2="52" y2="58"><stop stop-color="${color}"/><stop offset="1" stop-color="#ffffff" stop-opacity=".72"/></linearGradient></defs><path d="M16 8h23l11 11v37H16z" fill="url(#g)" opacity=".96"/><path d="M39 8v12h11" fill="#fff" opacity=".38"/><path d="M21 34h22M21 42h18" stroke="#1b2330" stroke-width="3" stroke-linecap="round" opacity=".54"/><text x="32" y="29" text-anchor="middle" font-family="Segoe UI,Arial" font-size="${kind === 'small' ? 13 : 15}" font-weight="800" fill="#1b2330" opacity=".78">${label}</text></svg>`
  const url = `data:image/svg+xml;utf8,${encodeURIComponent(svg)}`
  fallbackIconUrls.set(cacheKey, url)
  return url
}

function iconOf(item: LauncherItem) {
  if (item.isDir) return fileIconUrl('small', 'DIR', '#ffd166')
  const ext = item.extension.toLowerCase()
  return fileIconUrl('small', (ext || 'FILE').slice(0, 4).toUpperCase(), '#cbd5e1')
}

function quitApp() {
  Quit()
}
</script>

<template>
  <main class="shell" :class="{ 'is-expanded': expanded }" :style="shellStyle" @mouseenter="cancelCollapse" @mouseleave="scheduleCollapse">
    <div v-if="!expanded && hasFolders" class="side-poem side-poem-left-primary">{{ sidePoemLines[0] }}</div>
    <div v-if="!expanded && hasFolders && sidePoemLines[1]" class="side-poem side-poem-left-secondary">{{ sidePoemLines[1] }}</div>

    <section v-if="booting" class="empty glass-card booting">正在读取本地配置...</section>

    <section v-else-if="showSettings || !hasFolders" class="setup glass-card" :class="{ 'is-first-run': !hasFolders }">
      <button class="setup-close" title="退出" @click="quitApp">×</button>
      <div class="setup-panel-head">
        <div class="setup-icon">
          <img :src="appIcon" alt="jczhl Flyme Launcher" />
        </div>
        <div>
          <div class="setup-kicker">{{ hasFolders ? 'Preferences' : 'First Launch' }}</div>
          <h2>{{ hasFolders ? '启动器设置' : '选择扫描目录' }}</h2>
        </div>
      </div>
      <p>{{ hasFolders ? '管理目录和底部诗句，保存后会重新扫描。' : '添加一个或多个目录，启动器只合并扫描这些目录的当前层内容。' }}</p>
      <div class="settings-list">
        <div v-if="!settingsFolders.length" class="settings-empty">还没有选择目录</div>
        <div v-for="(settingFolder, index) in settingsFolders" :key="settingFolder" class="settings-row">
          <span>{{ settingFolder }}</span>
          <button @click="removeSettingsFolder(index)">×</button>
        </div>
      </div>
      <input v-model="settingsPoem" class="poem-input" maxlength="32" placeholder="自定义诗句" />
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
          <button
            v-for="item in activeItems"
            :key="`fly-${item.path}`"
            class="fly-item"
            title="左键打开 · 右键打开所在位置"
            @click="openItem(item)"
            @contextmenu="revealItem(item, $event)"
          >
            <span class="fly-icon">
              <img v-if="item.icon" :src="item.icon" :alt="item.name" />
              <img v-else :src="iconOf(item)" :alt="item.extension || item.name" />
            </span>
            <strong>{{ item.name }}</strong>
          </button>
        </template>
      </div>

      <div class="expanded-footer">
        <div class="footer-name">zhlhlf</div>
        <button class="footer-poem" title="退出应用" @click="quitApp">{{ poem }}</button>
      </div>
    </template>
  </main>
</template>
