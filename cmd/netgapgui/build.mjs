import { spawnSync } from 'node:child_process'
import { readFileSync, writeFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import path from 'node:path'

const projectDir = path.dirname(fileURLToPath(import.meta.url))
const configPath = path.join(projectDir, 'wails.json')
const originalConfig = readFileSync(configPath)
const config = JSON.parse(originalConfig.toString('utf8'))

const gitVersion = spawnSync('git', ['describe', '--tags', '--always', '--dirty'], {
  cwd: projectDir,
  encoding: 'utf8',
})
const appVersion = process.env.VITE_APP_VERSION?.trim()
  || (gitVersion.status === 0 ? gitVersion.stdout.trim() : '')
  || 'dev'
const productVersion = appVersion.match(/^v?(\d+\.\d+\.\d+)/)?.[1]
  || '0.0.0'

config.info.productVersion = productVersion

try {
  writeFileSync(configPath, `${JSON.stringify(config, null, 2)}\n`)
  const result = spawnSync('wails', ['build', ...process.argv.slice(2)], {
    cwd: projectDir,
    env: { ...process.env, VITE_APP_VERSION: appVersion },
    stdio: 'inherit',
    shell: process.platform === 'win32',
  })
  if (result.error) throw result.error
  process.exitCode = result.status ?? 1
} finally {
  writeFileSync(configPath, originalConfig)
}
