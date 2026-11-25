import { spawnSync } from 'node:child_process'
import { mkdirSync, readFileSync, writeFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import path from 'node:path'

const defaultProjectDir = path.dirname(fileURLToPath(import.meta.url))

function run(command, args, { spawn = spawnSync, ...options } = {}) {
  const result = spawn(command, args, {
    stdio: 'inherit',
    shell: process.platform === 'win32',
    ...options,
  })
  if (result.error) throw result.error
  if (result.status !== 0) {
    const error = new Error(`${command} ${args.join(' ')} failed`)
    error.exitCode = result.status ?? 1
    throw error
  }
  return result
}

function parseArgs(args) {
  const result = { tags: [], platform: '', skipBindings: false }
  for (let i = 0; i < args.length; i++) {
    const arg = args[i]
    if (arg === '-platform') result.platform = args[++i] ?? ''
    else if (arg.startsWith('-platform=')) result.platform = arg.slice(10)
    else if (arg === '-tags') result.tags.push(args[++i] ?? '')
    else if (arg.startsWith('-tags=')) result.tags.push(arg.slice(6))
    else if (arg === '-skipbindings') result.skipBindings = true
  }
  return result
}

export function build({ projectDir = defaultProjectDir, args = process.argv.slice(2), spawn = spawnSync, env = process.env } = {}) {
  const runBuild = (command, commandArgs, options = {}) => run(command, commandArgs, {
    spawn,
    cwd: options.cwd ?? projectDir,
    ...options,
  })
  const config = path.join(projectDir, 'build', 'config.yml')
  if (!readFileSync(config, 'utf8').trim()) throw new Error(`${config} is empty`)
  const gitVersion = runBuild('git', ['describe', '--tags', '--always', '--dirty'], { encoding: 'utf8', stdio: 'pipe' })
  const appVersion = env.VITE_APP_VERSION?.trim()
    || gitVersion?.stdout?.trim()
    || 'dev'
  const productVersion = appVersion.match(/^v?(\d+\.\d+\.\d+)/)?.[1] ?? '0.0.0'
  const { tags, platform, skipBindings } = parseArgs(args)
  const [goos, goarch] = platform.split('/')
  mkdirSync(path.join(projectDir, 'build', 'bin'), { recursive: true })
  runBuild('npm', ['ci'], { cwd: path.join(projectDir, 'frontend') })
  runBuild('npm', ['run', 'check'], { cwd: path.join(projectDir, 'frontend') })
  if (!skipBindings) runBuild('wails3', ['generate', 'bindings', '-clean', '-d', 'frontend/src/bindings'])
  runBuild('npm', ['run', 'compile'], {
    cwd: path.join(projectDir, 'frontend'),
    env: { ...env, VITE_APP_VERSION: appVersion },
  })
  if (goos === 'darwin') {
    mkdirSync(path.join(projectDir, 'build', 'generated', 'windows'), { recursive: true })
    mkdirSync(path.join(projectDir, 'build', 'generated', 'darwin'), { recursive: true })
    runBuild('wails3', ['generate', 'icons', '-input', 'build/appicon.png', '-windowsfilename', 'build/generated/windows/icon.ico', '-macfilename', 'build/generated/darwin/icons.icns'])
  }

  const cliArgs = ['build']
  if (tags.length) cliArgs.push('-tags', [...new Set(tags.flatMap((tag) => tag.split(',')))].join(','))
  cliArgs.push(`PRODUCT_VERSION=${productVersion}`)
  const buildEnv = { ...env, VITE_APP_VERSION: appVersion }
  if (goos) buildEnv.GOOS = goos
  if (goarch && goarch !== 'universal') buildEnv.GOARCH = goarch
  if (goos === 'windows') {
    const windowsGUIFlag = '-ldflags=-H=windowsgui'
    const goFlags = buildEnv.GOFLAGS?.trim()
    buildEnv.GOFLAGS = goFlags ? `${goFlags} ${windowsGUIFlag}` : windowsGUIFlag
  }

  if (goos === 'darwin' && goarch === 'universal') {
    for (const arch of ['amd64', 'arm64']) {
      runBuild('wails3', [...cliArgs, `GOOS=darwin`, `GOARCH=${arch}`, `OUTPUT=build/bin/netgapgui-${arch}`], { env: buildEnv })
    }
    runBuild('lipo', ['-create', '-output', 'build/bin/netgapgui', 'build/bin/netgapgui-amd64', 'build/bin/netgapgui-arm64'])
  } else {
    runBuild('wails3', cliArgs, { env: buildEnv })
  }

  if (goos === 'darwin') {
    const appDir = path.join(projectDir, 'build', 'bin', 'netgapgui.app')
    const contents = path.join(appDir, 'Contents')
    const macOS = path.join(contents, 'MacOS')
    const resources = path.join(contents, 'Resources')
    mkdirSync(macOS, { recursive: true })
    mkdirSync(resources, { recursive: true })
    runBuild('node', ['-e', 'const fs=require("fs");fs.copyFileSync("build/bin/netgapgui","build/bin/netgapgui.app/Contents/MacOS/netgapgui")'])
    runBuild('node', ['-e', 'const fs=require("fs");fs.copyFileSync("build/generated/darwin/icons.icns","build/bin/netgapgui.app/Contents/Resources/icons.icns")'])
    writeFileSync(path.join(contents, 'Info.plist'), `<?xml version="1.0" encoding="UTF-8"?>\n<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">\n<plist version="1.0"><dict><key>CFBundleExecutable</key><string>netgapgui</string><key>CFBundleIdentifier</key><string>io.netgap.netgapgui</string><key>CFBundleName</key><string>NetGapGUI</string><key>CFBundlePackageType</key><string>APPL</string><key>CFBundleIconFile</key><string>icons</string><key>CFBundleVersion</key><string>${productVersion}</string><key>CFBundleShortVersionString</key><string>${productVersion}</string><key>LSMinimumSystemVersion</key><string>12.0</string></dict></plist>\n`)
  }
  return 0
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try { process.exitCode = build() } catch (error) {
    console.error(error.message)
    process.exitCode = error.exitCode ?? 1
  }
}
