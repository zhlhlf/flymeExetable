import { copyFileSync, existsSync, mkdirSync, rmSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = dirname(dirname(fileURLToPath(import.meta.url)))
const source = join(root, 'appicon-rounded.png')
const buildDir = join(root, 'build')
const windowsIcon = join(buildDir, 'windows', 'icon.ico')
const buildIcon = join(buildDir, 'appicon.png')

if (!existsSync(source)) {
  throw new Error(`Missing icon source: ${source}`)
}

mkdirSync(buildDir, { recursive: true })
copyFileSync(source, buildIcon)
rmSync(windowsIcon, { force: true })

console.log(`Prepared Wails icon: ${buildIcon}`)
