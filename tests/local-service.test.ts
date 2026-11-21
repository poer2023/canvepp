import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { spawn, type ChildProcess } from 'node:child_process';
import { createServer } from 'node:net';
import { createServer as createHttpServer, type Server } from 'node:http';
import { mkdtemp, mkdir, readFile, writeFile, rm, realpath } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { ensureLocalService, serviceRootId } from '../scripts/local-service.mjs';

const launcherUrl = pathToFileURL(fileURLToPath(new URL('../scripts/local-service.mjs', import.meta.url))).href;
const pause = (ms: number) => new Promise(resolve => setTimeout(resolve, ms));
const fixture = `
import http from 'node:http';
import fs from 'node:fs';
import crypto from 'node:crypto';
import path from 'node:path';
const arg = process.argv.indexOf('--port');
const port = Number(arg >= 0 ? process.argv[arg + 1] : process.argv[2]);
const root = fs.realpathSync(process.cwd());
const record = path.join(root, 'starts.jsonl');
fs.appendFileSync(record, JSON.stringify({pid:process.pid,args:process.argv.slice(2),instanceId:process.env.CANVEPP_INSTANCE_ID})+'\\n');
console.log('fixture started', process.pid);
if (process.env.FIXTURE_MODE === 'exit') process.exit(9);
const server = http.createServer((req, res) => {
  fs.appendFileSync(path.join(root, 'requests.jsonl'), JSON.stringify({url:req.url,origin:req.headers.origin})+'\\n');
  if (process.env.FIXTURE_MODE === 'hang') return;
  if (req.url !== '/api/health') {res.writeHead(404).end();return;}
  res.setHeader('Content-Type', 'application/json');
  if (process.env.FIXTURE_MODE === 'malformed') {res.end('{');return;}
  if (process.env.FIXTURE_MODE === 'oversized') {res.end('x'.repeat(17000));return;}
  res.end(JSON.stringify({service:process.env.FIXTURE_SERVICE || 'canvepp',protocolVersion:1,
    rootId:process.env.FIXTURE_ROOT || crypto.createHash('sha256').update(root).digest('hex'),
    instanceId:process.env.CANVEPP_INSTANCE_ID || 'fixture-external',pid:process.pid}));
});
setTimeout(() => server.listen(port,'127.0.0.1'),Number(process.env.FIXTURE_DELAY || 0));
process.on('SIGTERM', () => {if(process.env.FIXTURE_IGNORE_TERM === 'yes')return;server.close();process.exit(0);});
`;

let root: string;
let script: string;
let port: number;
let baseUrl: string;
const children = new Set<ChildProcess>();
const servers = new Set<Server>();

async function freePort() {
  const server = createServer();
  await new Promise<void>((resolve, reject) => { server.once('error', reject); server.listen(0, '127.0.0.1', resolve); });
  const address = server.address();
  if (!address || typeof address === 'string') throw new Error('Missing fixture port');
  const value = address.port;
  await new Promise<void>(resolve => server.close(() => resolve()));
  return value;
}

async function records(name: string): Promise<Array<Record<string, any>>> {
  try { return (await readFile(path.join(root, name), 'utf8')).trim().split('\n').filter(Boolean).map(line => JSON.parse(line)); }
  catch (error) { if ((error as NodeJS.ErrnoException).code === 'ENOENT') return []; throw error; }
}

function launch(env: Record<string, string> = {}) {
  return { command: process.execPath, args: [script, String(port)], env };
}

function options(env: Record<string, string> = {}) {
  return { baseUrl, projectRoot: root, launch: launch(env), startupTimeoutMs: 2500, pollIntervalMs: 25, probeTimeoutMs: 100 };
}

function alive(pid: number) {
  try { process.kill(pid, 0); return true; } catch { return false; }
}

async function eventually(check: () => Promise<boolean> | boolean, timeout = 2500) {
  const deadline = Date.now() + timeout;
  while (Date.now() < deadline) { if (await check()) return; await pause(20); }
  throw new Error('Fixture did not reach expected state');
}

async function externalFixture(env: Record<string, string> = {}) {
  const child = spawn(process.execPath, [script, String(port)], { cwd: root, env: { ...process.env, ...env }, stdio: 'ignore' });
  children.add(child);
  await eventually(async () => { try { return (await fetch(`${baseUrl}/api/health`, { signal: AbortSignal.timeout(100) })).ok; } catch { return false; } });
  return child;
}

function worker(opts: ReturnType<typeof options>): Promise<any> {
  const code = `import {ensureLocalService} from ${JSON.stringify(launcherUrl)};try {console.log(JSON.stringify(await ensureLocalService(${JSON.stringify(opts)})))} catch(error) {console.error(JSON.stringify({code:error.code,message:error.message}));process.exitCode=1}`;
  return new Promise((resolve, reject) => {
    const child = spawn(process.execPath, ['--input-type=module', '-e', code], { cwd: root, stdio: ['ignore', 'pipe', 'pipe'] });
    children.add(child);
    let stdout = '', stderr = '';
    child.stdout!.on('data', chunk => { stdout += chunk; });
    child.stderr!.on('data', chunk => { stderr += chunk; });
    child.on('error', reject);
    child.on('close', code => code === 0 ? resolve(JSON.parse(stdout.trim())) : reject(new Error(`Worker failed ${code}: ${stderr}`)));
  });
}

beforeEach(async () => {
  root = await realpath(await mkdtemp(path.join(tmpdir(), 'canvepp-launch-test-')));
  script = path.join(root, 'fixture.mjs');
  await writeFile(script, fixture);
  port = await freePort();
  if (port === 3210) throw new Error('Tests must never touch the app port');
  baseUrl = `http://127.0.0.1:${port}`;
});

afterEach(async () => {
  for (const child of children) if (child.exitCode === null && child.signalCode === null) child.kill('SIGTERM');
  children.clear();
  const owned = await records('starts.jsonl');
  for (const item of owned) { try { process.kill(item.pid as number, 'SIGTERM'); } catch { /* already stopped by launcher */ } }
  await Promise.all([...servers].map(server => new Promise<void>(resolve => server.close(() => resolve()))));
  servers.clear();
  await eventually(() => owned.every(item => !alive(item.pid as number))).catch(() => {});
  await rm(root, { recursive: true, force: true });
});

describe('on-demand local canvas launcher', () => {
  it('cold-starts a loopback fixture, returns identity and logs, and only probes health', async () => {
    const result = await ensureLocalService(options());
    expect(result).toMatchObject({ url: baseUrl, started: true, status: 'started', rootId: await serviceRootId(root) });
    expect(result.pid).toBeGreaterThan(0);
    expect(result.launcherPid).toBe(result.pid);
    expect(result.instanceId).toBeTruthy();
    expect(result.logFile).toBe(path.join(root, '.data/runtime/server.log'));
    expect(await readFile(result.logFile, 'utf8')).toContain(`pid=${result.pid}`);
    expect(await readFile(result.logFile, 'utf8')).toContain('fixture started');
    expect((await records('starts.jsonl'))).toHaveLength(1);
    const requests = await records('requests.jsonl');
    expect(requests.length).toBeGreaterThan(0);
    expect(requests.every(request => request.url === '/api/health' && request.origin === baseUrl)).toBe(true);
    const state = JSON.parse(await readFile(path.join(root, `.data/runtime/service-${port}.json`), 'utf8'));
    expect(state).toMatchObject({ pid: result.pid, instanceId: result.instanceId, rootId: result.rootId });
  });

  it('reuses an identified instance without another spawn', async () => {
    const first = await ensureLocalService(options());
    const second = await ensureLocalService(options({ FIXTURE_MODE: 'exit' }));
    expect(second).toMatchObject({ pid: first.pid, instanceId: first.instanceId, started: false, status: 'reused' });
    expect(await records('starts.jsonl')).toHaveLength(1);
  });

  it('coordinates separate MCP processes so simultaneous cold opens spawn once', async () => {
    const outcomes = await Promise.all(Array.from({ length: 4 }, () => worker(options({ FIXTURE_DELAY: '220' }))));
    expect(new Set(outcomes.map(outcome => outcome.pid)).size).toBe(1);
    expect(outcomes.filter(outcome => outcome.started)).toHaveLength(1);
    expect(await records('starts.jsonl')).toHaveLength(1);
  }, 10_000);

  it('reclaims a dead lock once under concurrent callers without killing a PID', async () => {
    const dead = spawn(process.execPath, ['-e', 'process.exit(0)']);
    await new Promise<void>(resolve => dead.once('close', () => resolve()));
    const runtime = path.join(root, '.data/runtime');
    await mkdir(runtime, { recursive: true });
    await writeFile(path.join(runtime, `startup-${port}.lock`), JSON.stringify({ pid: dead.pid, nonce: 'old-dead-owner', rootId: await serviceRootId(root) }));
    const outcomes = await Promise.all(Array.from({ length: 4 }, () => worker(options({ FIXTURE_DELAY: '180' }))));
    expect(new Set(outcomes.map(outcome => outcome.pid)).size).toBe(1);
    expect(await records('starts.jsonl')).toHaveLength(1);
  }, 10_000);

  it('waits for a previously recorded child that outlived its launcher', async () => {
    const child = spawn(process.execPath, [script, String(port)], { cwd: root, env: { ...process.env, FIXTURE_DELAY: '300', CANVEPP_INSTANCE_ID: 'surviving-child' }, stdio: 'ignore' });
    children.add(child);
    const runtime = path.join(root, '.data/runtime');
    await mkdir(runtime, { recursive: true });
    await writeFile(path.join(runtime, `service-${port}.json`), JSON.stringify({ rootId: await serviceRootId(root), instanceId: 'surviving-child', launcherPid: child.pid }));
    const result = await ensureLocalService(options());
    expect(result).toMatchObject({ started: false, pid: child.pid, instanceId: 'surviving-child' });
    expect(await records('starts.jsonl')).toHaveLength(1);
  });

  it('does not take a live lock or terminate its owner when waiting times out', async () => {
    const runtime = path.join(root, '.data/runtime');
    await mkdir(runtime, { recursive: true });
    const lock = path.join(runtime, `startup-${port}.lock`);
    const owner = { pid: process.pid, nonce: 'live-test-owner', rootId: await serviceRootId(root) };
    await writeFile(lock, JSON.stringify(owner));
    await expect(ensureLocalService({ ...options(), startupTimeoutMs: 180 })).rejects.toMatchObject({ code: 'STARTUP_TIMEOUT' });
    expect(JSON.parse(await readFile(lock, 'utf8'))).toEqual(owner);
    expect(await records('starts.jsonl')).toHaveLength(0);
  });

  it.each([
    ['unrelated product', { FIXTURE_SERVICE: 'another-app' }, 'PORT_IN_USE'],
    ['different project', { FIXTURE_ROOT: 'another-root' }, 'WRONG_PROJECT'],
    ['malformed identity', { FIXTURE_MODE: 'malformed' }, 'PORT_IN_USE'],
    ['unbounded identity', { FIXTURE_MODE: 'oversized' }, 'PORT_IN_USE'],
  ])('rejects %s on the port and preserves that process', async (_label, env, code) => {
    const child = await externalFixture(env as Record<string, string>);
    await expect(ensureLocalService(options())).rejects.toMatchObject({ code });
    expect(alive(child.pid!)).toBe(true);
    expect(await records('starts.jsonl')).toHaveLength(1);
  });

  it('rejects an occupied port with no health endpoint and leaves it listening', async () => {
    const server = createHttpServer((_request, response) => response.writeHead(404).end());
    servers.add(server);
    await new Promise<void>(resolve => server.listen(port, '127.0.0.1', resolve));
    await expect(ensureLocalService(options())).rejects.toMatchObject({ code: 'PORT_IN_USE' });
    expect(server.listening).toBe(true);
    expect(await records('starts.jsonl')).toHaveLength(0);
  });

  it.each(['https://127.0.0.1:4444', 'http://localhost:4444', 'http://192.168.1.2:4444', 'http://fixture@127.0.0.1:4444', 'http://127.0.0.1:0', 'http://127.0.0.1:4444/path', 'http://127.0.0.1:4444/?extra=yes'])('refuses nonlocal or ambiguous address %s', async url => {
    await expect(ensureLocalService({ ...options(), baseUrl: url })).rejects.toMatchObject({ code: 'INVALID_URL' });
    expect(await records('starts.jsonl')).toHaveLength(0);
  });

  it('bounds readiness waiting and stops its own newly spawned process', async () => {
    await expect(ensureLocalService({ ...options({ FIXTURE_DELAY: '20000' }), startupTimeoutMs: 250 })).rejects.toMatchObject({ code: 'STARTUP_TIMEOUT' });
    const starts = await records('starts.jsonl');
    expect(starts).toHaveLength(1);
    await eventually(() => !alive(starts[0].pid));
    await expect(readFile(path.join(root, `.data/runtime/service-${port}.json`))).rejects.toMatchObject({ code: 'ENOENT' });
    await expect(readFile(path.join(root, `.data/runtime/startup-${port}.lock`))).rejects.toMatchObject({ code: 'ENOENT' });
  });

  it('escalates only its own timed-out child when graceful termination is ignored', async () => {
    await expect(ensureLocalService({ ...options({ FIXTURE_DELAY: '20000', FIXTURE_IGNORE_TERM: 'yes' }), startupTimeoutMs: 250 })).rejects.toMatchObject({ code: 'STARTUP_TIMEOUT' });
    const starts = await records('starts.jsonl');
    expect(starts).toHaveLength(1);
    expect(alive(starts[0].pid)).toBe(false);
  });

  it('reports a nonexistent executable without leaving a lock or valid service record', async () => {
    await expect(ensureLocalService({ ...options(), launch: { command: path.join(root, 'missing-executable'), args: [] } })).rejects.toMatchObject({ code: 'STARTUP_FAILED' });
    await expect(readFile(path.join(root, `.data/runtime/startup-${port}.lock`))).rejects.toMatchObject({ code: 'ENOENT' });
    await expect(readFile(path.join(root, `.data/runtime/service-${port}.json`))).rejects.toMatchObject({ code: 'ENOENT' });
  });

  it('reports a child exit instead of retrying or submitting work', async () => {
    await expect(ensureLocalService(options({ FIXTURE_MODE: 'exit' }))).rejects.toMatchObject({ code: 'STARTUP_FAILED', exitCode: 9 });
    expect(await records('starts.jsonl')).toHaveLength(1);
    expect(await records('requests.jsonl')).toHaveLength(0);
  });

  it('requires installed dependencies and never attempts npm installation', async () => {
    const { launch: _fixture, ...defaultOptions } = options();
    await expect(ensureLocalService(defaultOptions)).rejects.toMatchObject({ code: 'DEPENDENCIES_MISSING' });
    expect(await records('starts.jsonl')).toHaveLength(0);
  });

  it('uses the current Node and exact loopback Next development arguments by default', async () => {
    const next = path.join(root, 'node_modules/next/dist/bin/next');
    await mkdir(path.dirname(next), { recursive: true });
    await writeFile(path.join(root, 'package.json'), JSON.stringify({ type: 'module' }));
    await writeFile(next, fixture);
    const { launch: _fixture, ...defaultOptions } = options();
    const result = await ensureLocalService(defaultOptions);
    expect(result.command).toEqual({ file: process.execPath, args: [next, 'dev', '--hostname', '127.0.0.1', '--port', String(port)] });
    expect((await records('starts.jsonl'))[0].args).toEqual(['dev', '--hostname', '127.0.0.1', '--port', String(port)]);
  });
});
