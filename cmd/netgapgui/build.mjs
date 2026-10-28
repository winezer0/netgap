import { spawnSync } from 'node:child_process'
import { readFileSync, writeFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import path from 'node:path'

const defaultProjectDir = path.dirname(fileURLToPath(import.meta.url))

export function build({ projectDir = defaultProjectDir, args = process.argv.slice(2), spawn = spawnSync } = {}) {
  const configPath = path.join(projectDir, 'wails.json')
  const originalConfig = readFileSync(configPath)
  if (!originalConfig.toString('utf8').trim()) {
    throw new Error(`${configPath} is empty; restore a valid Wails configuration before building`)
  }
  let config
  try {
    config = JSON.parse(originalConfig.toString('utf8'))
  } catch (error) {
    throw new Error(`Invalid JSON in ${configPath}: ${error.message}`, { cause: error })
  }
  if (!config.info || typeof config.info !== 'object') {
    throw new Error(`${configPath} must contain an "info" object`)
  }

  // Check before Wails can package anything, including builds using -s.
  for (const npmArgs of [['ci'], ['run', 'check']]) {
    const result = spawn('npm', npmArgs, {
      cwd: path.join(projectDir, 'frontend'),
      stdio: 'inherit',
      shell: process.platform === 'win32',
    })
    if (result.error) throw result.error
    if (result.status !== 0) {
      const error = new Error(`npm ${npmArgs.join(' ')} failed; GUI packaging stopped`)
      error.exitCode = result.status ?? 1
      throw error
    }
  }

  const gitVersion = spawn('git', ['describe', '--tags', '--always', '--dirty'], {
    cwd: projectDir,
    encoding: 'utf8',
  })
  const appVersion = process.env.VITE_APP_VERSION?.trim()
    || (gitVersion.status === 0 ? gitVersion.stdout.trim() : '')
    || 'dev'
  const productVersion = appVersion.match(/^v?(\d+\.\d+\.\d+)/)?.[1]
    || '0.0.0'

  config.info.productVersion = productVersion
  // Direct Wails builds use npm run build, which includes the same checks.
  config['frontend:install'] = ''
  config['frontend:build'] = 'npm run compile'

  try {
    writeFileSync(configPath, `${JSON.stringify(config, null, 2)}\n`)
    const result = spawn('wails', ['build', ...args], {
      cwd: projectDir,
      env: { ...process.env, VITE_APP_VERSION: appVersion },
      stdio: 'inherit',
      shell: process.platform === 'win32',
    })
    if (result.error) throw result.error
    return result.status ?? 1
  } finally {
    writeFileSync(configPath, originalConfig)
  }
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    process.exitCode = build()
  } catch (error) {
    console.error(error.message)
    process.exitCode = error.exitCode ?? 1
  }
}
