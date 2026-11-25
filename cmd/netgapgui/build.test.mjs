import { test } from 'node:test'
import assert from 'node:assert/strict'
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import path from 'node:path'
import { build } from './build.mjs'

function fixture(t, resultFor = () => ({ status: 0, stdout: 'v1.2.3' })) {
  const projectDir = mkdtempSync(path.join(tmpdir(), 'netgap-build-gate-'))
  mkdirSync(path.join(projectDir, 'build'))
  mkdirSync(path.join(projectDir, 'frontend'))
  writeFileSync(path.join(projectDir, 'build', 'config.yml'), "version: '3'\n")
  t.after(() => rmSync(projectDir, { recursive: true, force: true }))
  const calls = []
  const spawn = (command, args, options) => {
    calls.push({ command, args, options })
    return resultFor(command, args, options)
  }
  return { projectDir, calls, spawn }
}

for (const failedStage of ['ci', 'check']) {
  test(`${failedStage} failure stops before Wails packaging`, (t) => {
    const f = fixture(t, (command, args) => ({ status: command === 'npm' && args.includes(failedStage) ? 7 : 0 }))
    assert.throws(() => build({ ...f, args: ['-platform', 'windows/amd64'] }), (error) => error.exitCode === 7)
    assert.equal(f.calls.some(({ command }) => command === 'wails3'), false)
  })
}

test('frontend checks and binding generation precede Wails v3 build', (t) => {
  const f = fixture(t)
  assert.equal(build({ ...f, args: ['-tags', 'dezip_full', '-platform', 'linux/amd64'], env: {} }), 0)
  assert.deepEqual(f.calls.map(({ command, args }) => [command, args]), [
    ['git', ['describe', '--tags', '--always', '--dirty']],
    ['npm', ['ci']], ['npm', ['run', 'check']],
    ['wails3', ['generate', 'bindings', '-clean', '-d', 'frontend/src/bindings']],
    ['npm', ['run', 'compile']],
    ['wails3', ['build', '-tags', 'dezip_full', 'PRODUCT_VERSION=1.2.3']],
  ])
  assert.equal(f.calls.at(-1).options.env.GOOS, 'linux')
  assert.equal(f.calls.at(-1).options.env.GOARCH, 'amd64')
})

test('Windows builds hide the console while preserving configured Go flags', (t) => {
  const f = fixture(t)
  build({ ...f, args: ['-platform', 'windows/amd64'], env: { GOFLAGS: '-tags=dezip_full' } })
  const wailsBuild = f.calls.find(({ command, args }) => command === 'wails3' && args[0] === 'build')
  assert.equal(wailsBuild.options.env.GOFLAGS, '-tags=dezip_full -ldflags=-H=windowsgui')
})

test('Wails v3 build failures preserve their exit status', (t) => {
  const f = fixture(t, (command, args) => ({ status: command === 'wails3' && args[0] === 'build' ? 9 : 0 }))
  assert.throws(() => build({ ...f, args: [] }), (error) => error.exitCode === 9)
})

test('universal macOS build compiles both architectures then creates a universal binary', (t) => {
  const f = fixture(t)
  build({ ...f, args: ['-platform', 'darwin/universal'], env: {} })
  const wailsBuilds = f.calls.filter(({ command, args }) => command === 'wails3' && args[0] === 'build')
  assert.equal(wailsBuilds.length, 2)
  assert.ok(wailsBuilds[0].args.includes('GOARCH=amd64'))
  assert.ok(wailsBuilds[1].args.includes('GOARCH=arm64'))
  assert.ok(f.calls.some(({ command }) => command === 'lipo'))
})
