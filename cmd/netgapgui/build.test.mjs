import { test } from 'node:test'
import assert from 'node:assert/strict'
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import path from 'node:path'
import { build } from './build.mjs'

function fixture(t, resultFor = () => ({ status: 0, stdout: 'v1.2.3' })) {
  const projectDir = mkdtempSync(path.join(tmpdir(), 'netgap-build-gate-'))
  t.after(() => rmSync(projectDir, { recursive: true, force: true }))
  const configPath = path.join(projectDir, 'wails.json')
  const original = Buffer.from('{"info":{"productVersion":"0.1.0"},"frontend:install":"npm ci","frontend:build":"npm run build"}\r\n')
  writeFileSync(configPath, original)
  const calls = []
  const spawn = (command, args, options) => {
    calls.push({ command, args, options })
    return resultFor(command, args, options, configPath)
  }
  return { projectDir, configPath, original, calls, spawn }
}

for (const failedStage of ['ci', 'check']) {
  test(`${failedStage} failure prevents packaging even with -s`, (t) => {
    const f = fixture(t, (command, args) => ({ status: command === 'npm' && args.includes(failedStage) ? 7 : 0 }))
    assert.throws(() => build({ ...f, args: ['-s'] }), (error) => error.exitCode === 7)
    assert.equal(f.calls.some(({ command }) => command === 'wails'), false)
    assert.deepEqual(readFileSync(f.configPath), f.original)
  })
}

test('checks precede packaging and Wails does not repeat them', (t) => {
  const f = fixture(t, (command, _args, _options, configPath) => {
    if (command === 'wails') {
      const config = JSON.parse(readFileSync(configPath, 'utf8'))
      assert.equal(config['frontend:install'], '')
      assert.equal(config['frontend:build'], 'npm run compile')
    }
    return { status: 0, stdout: 'v1.2.3' }
  })
  assert.equal(build({ ...f, args: ['-s', '-platform', 'windows/amd64'] }), 0)
  assert.deepEqual(f.calls.map(({ command, args }) => [command, args]), [
    ['npm', ['ci']], ['npm', ['run', 'check']],
    ['git', ['describe', '--tags', '--always', '--dirty']],
    ['wails', ['build', '-s', '-platform', 'windows/amd64']],
  ])
  assert.deepEqual(readFileSync(f.configPath), f.original)
})

test('Wails failure preserves exit status and restores configuration', (t) => {
  const f = fixture(t, (command) => ({ status: command === 'wails' ? 9 : 0, stdout: 'dev' }))
  assert.equal(build({ ...f, args: [] }), 9)
  assert.deepEqual(readFileSync(f.configPath), f.original)
})

test('Wails startup errors also restore configuration', (t) => {
  const f = fixture(t, (command) => command === 'wails'
    ? { error: new Error('Wails not installed') }
    : { status: 0, stdout: 'dev' })
  assert.throws(() => build({ ...f, args: [] }), /Wails not installed/)
  assert.deepEqual(readFileSync(f.configPath), f.original)
})
