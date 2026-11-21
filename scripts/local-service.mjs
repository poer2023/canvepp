import { spawn } from 'node:child_process';
import { createHash, randomUUID } from 'node:crypto';
import { existsSync, openSync, closeSync, appendFileSync, writeFileSync, renameSync } from 'node:fs';
import { mkdir, readFile, writeFile, link, unlink, realpath } from 'node:fs/promises';
import net from 'node:net';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const defaultRoot = fileURLToPath(new URL('..', import.meta.url));
const delay = (ms) => new Promise(resolve => setTimeout(resolve, ms));

export class LocalServiceError extends Error {
  constructor(code, message, details = {}) { super(message); this.name = 'LocalServiceError'; this.code = code; Object.assign(this, details); }
}

/** @param {string} projectRoot */
export async function serviceRootId(projectRoot) {
  return createHash('sha256').update(await realpath(projectRoot)).digest('hex');
}

function localUrl(value) {
  let url;
  try { url = new URL(value); } catch { throw new LocalServiceError('INVALID_URL', '画布地址必须是本机 http://127.0.0.1:<端口>。'); }
  if (url.protocol !== 'http:' || url.hostname !== '127.0.0.1' || url.username || url.href.includes('@') || url.pathname !== '/' || url.search || url.hash || Number(url.port || 80) < 1) {
    throw new LocalServiceError('INVALID_URL', '启动器仅允许 http://127.0.0.1:<端口>，不能使用远程地址或额外路径。');
  }
  return url;
}

function alive(pid) {
  if (!Number.isInteger(pid) || pid <= 0) return false;
  try { process.kill(pid, 0); return true; } catch (error) { return error?.code === 'EPERM'; }
}

async function readJson(file) {
  try { return JSON.parse(await readFile(file, 'utf8')); }
  catch (error) { if (error?.code === 'ENOENT') return undefined; throw new LocalServiceError('INVALID_RUNTIME_STATE', '启动状态文件无法读取，请检查本地运行目录。'); }
}

/** Read only this small, unauthenticated identity endpoint. Never bootstrap a session or project. */
async function inspect(base, rootId, timeoutMs) {
  let response;
  try {
    response = await fetch(new URL('/api/health', base), { headers: { Origin: base.origin }, redirect: 'error', signal: AbortSignal.timeout(timeoutMs) });
  } catch { return undefined; }
  if (!response.ok) throw new LocalServiceError('PORT_IN_USE', `端口 ${base.port || 80} 上已有服务，但未提供可识别的 Canvepp 健康接口；未启动新进程。`);
  let health;
  try {
    const reader = response.body?.getReader();
    const chunks = []; let total = 0;
    if (!reader) throw new Error('Missing body');
    while (true) { const { done, value } = await reader.read(); if (done) break; total += value.length; if (total > 16 * 1024) { await reader.cancel(); throw new Error('Oversized health response'); } chunks.push(value); }
    health = JSON.parse(Buffer.concat(chunks).toString('utf8'));
  } catch { throw new LocalServiceError('PORT_IN_USE', '端口上的服务返回了无效健康信息；未将其当作 Canvepp 复用。'); }
  if (health?.service !== 'canvepp' || health.protocolVersion !== 1 || !Number.isInteger(health.pid) || health.pid <= 0 || typeof health.instanceId !== 'string' || !health.instanceId || health.instanceId.length > 160) {
    throw new LocalServiceError('PORT_IN_USE', '端口上的服务不是受支持的 Canvepp 实例；未启动或停止任何已有服务。');
  }
  if (health.rootId !== rootId) throw new LocalServiceError('WRONG_PROJECT', '该端口运行的是另一份 Canvepp 项目；请使用其他端口或先停止那份服务。');
  return health;
}

async function portIsFree(port) {
  return new Promise((resolve, reject) => {
    const probe = net.createServer();
    probe.unref();
    probe.once('error', error => error.code === 'EADDRINUSE' ? resolve(false) : reject(error));
    probe.listen({ host: '127.0.0.1', port, exclusive: true }, () => probe.close(() => resolve(true)));
  });
}

// Publish a fully written owner record atomically. A visible lock is never an
// empty file left between open("wx") and write(), even if the caller crashes.
async function acquire(lockFile, owner) {
  const candidate = `${lockFile}.${owner.pid}.${owner.nonce}.candidate`;
  await writeFile(candidate, JSON.stringify(owner), { flag: 'wx', mode: 0o600 });
  try { await link(candidate, lockFile); return true; }
  catch (error) { if (error?.code === 'EEXIST') return false; throw error; }
  finally { await unlink(candidate).catch(() => {}); }
}

async function release(lockFile, nonce) {
  const owner = await readJson(lockFile);
  if (owner?.nonce === nonce && owner.pid === process.pid) await unlink(lockFile).catch(error => { if (error?.code !== 'ENOENT') throw error; });
}

async function reapDeadLock(lockFile, rootId) {
  const owner = await readJson(lockFile);
  if (!owner) return;
  if (owner.rootId !== rootId || typeof owner.nonce !== 'string' || !/^[a-zA-Z0-9-]{1,160}$/.test(owner.nonce) || !Number.isInteger(owner.pid) || owner.pid <= 0) {
    throw new LocalServiceError('INVALID_RUNTIME_STATE', '已有启动锁的归属无法验证；启动器不会删除不明锁或停止不明进程。');
  }
  if (alive(owner.pid)) return;
  // Serialize stale-lock removal for this exact generation. Reapers never
  // remove each other's claims: a crashed reaper is skipped by adding a new
  // numbered claim. This avoids the read/check/unlink race where a second
  // stale reaper could unlink a freshly acquired startup lock.
  const claims = [];
  const reaper = { pid: process.pid, nonce: randomUUID(), rootId, target: owner.nonce };
  for (let index = 0; index < 64; index++) {
    const claimFile = `${lockFile}.${owner.nonce}.reap-${index}`;
    claims.push(claimFile);
    if (await acquire(claimFile, reaper)) {
      try {
        const current = await readJson(lockFile);
        if (current?.nonce === owner.nonce && current.pid === owner.pid && !alive(current.pid)) {
          await unlink(lockFile).catch(error => { if (error?.code !== 'ENOENT') throw error; });
        }
      } finally {
        // Removal is complete before claims are retired. Any delayed reaper
        // still compares the original nonce, so it cannot remove a new owner.
        await Promise.all(claims.map(file => unlink(file).catch(() => {})));
      }
      return;
    }
    const existing = await readJson(claimFile);
    // The active reaper may have just completed and retired the claim.
    if (!existing) return;
    if (existing.rootId !== rootId || existing.target !== owner.nonce || !Number.isInteger(existing.pid) || existing.pid <= 0) {
      throw new LocalServiceError('INVALID_RUNTIME_STATE', '启动锁回收记录的归属无法验证；未删除不明状态。');
    }
    if (alive(existing.pid)) return;
  }
  throw new LocalServiceError('INVALID_RUNTIME_STATE', '启动锁的历史回收次数异常，请检查本地运行目录。');
}

function atomicState(file, value) {
  const temporary = `${file}.${process.pid}.${randomUUID()}.tmp`;
  writeFileSync(temporary, JSON.stringify(value, null, 2), { flag: 'wx', mode: 0o600 });
  renameSync(temporary, file);
}

/** Only called with the ChildProcess object created by this invocation. */
async function stopOwnedChild(child) {
  if (!child?.pid) return;
  const running = () => {
    if (process.platform === 'win32') return child.exitCode === null && child.signalCode === null;
    try { process.kill(-child.pid, 0); return true; } catch (error) { return error?.code === 'EPERM'; }
  };
  const signal = (value) => {
    try { if (process.platform !== 'win32') process.kill(-child.pid, value); else if (running()) child.kill(value); }
    catch { try { if (child.exitCode === null && child.signalCode === null) child.kill(value); } catch { /* already gone */ } }
  };
  const waitForExit = async (ms) => {
    const deadline = Date.now() + ms;
    while (running() && Date.now() < deadline) await delay(20);
  };
  // The detached group belongs to this spawn, including a Next worker even if
  // its launcher has already exited. Never signal a PID loaded from disk.
  signal('SIGTERM');
  await waitForExit(600);
  if (running()) { signal('SIGKILL'); await waitForExit(400); }

}

/**
 * @typedef {{command:string,args:string[],cwd?:string,env?:Record<string,string>}} LaunchCommand
 * @typedef {{baseUrl?:string,projectRoot?:string,startupTimeoutMs?:number,pollIntervalMs?:number,probeTimeoutMs?:number,launch?:LaunchCommand}} ServiceOptions
 * @typedef {{url:string,pid:number,launcherPid?:number,started:boolean,status:'started'|'reused',rootId:string,instanceId:string,logFile:string,command?:{file:string,args:string[]}}} ServiceInfo
 */

/**
 * Reuse an identified local canvas, or start this project's Next development
 * server on demand. No shell, dependency installation, session creation,
 * generation, or project mutation occurs here. Custom launch is a trusted
 * caller option for isolated fixture tests; it must not be exposed as MCP input.
 * @param {ServiceOptions} [options]
 * @returns {Promise<ServiceInfo>}
 */
export async function ensureLocalService(options = {}) {
  const base = localUrl(options.baseUrl ?? 'http://127.0.0.1:3210');
  const root = await realpath(options.projectRoot ?? defaultRoot);
  const rootId = await serviceRootId(root);
  const runtimeDir = path.join(root, '.data', 'runtime');
  const logFile = path.join(runtimeDir, 'server.log');
  const port = Number(base.port || 80);
  const lockFile = path.join(runtimeDir, `startup-${port}.lock`);
  const stateFile = path.join(runtimeDir, `service-${port}.json`);
  const startupTimeoutMs = options.startupTimeoutMs ?? 45_000;
  const pollIntervalMs = options.pollIntervalMs ?? 200;
  const probeTimeoutMs = options.probeTimeoutMs ?? 1200;
  if (!Number.isFinite(startupTimeoutMs) || startupTimeoutMs < 100 || startupTimeoutMs > 120_000 || !Number.isFinite(pollIntervalMs) || pollIntervalMs < 20 || pollIntervalMs > 2000 || !Number.isFinite(probeTimeoutMs) || probeTimeoutMs < 50 || probeTimeoutMs > 5000) {
    throw new LocalServiceError('INVALID_OPTIONS', '启动、轮询或探活超时时间无效。');
  }
  const info = (health, started, state) => ({ url: base.origin, pid: health.pid, launcherPid: state?.instanceId === health.instanceId ? state.launcherPid : undefined, started, status: started ? 'started' : 'reused', rootId, instanceId: health.instanceId, logFile, command: state?.instanceId === health.instanceId ? state.command : undefined });
  const first = await inspect(base, rootId, probeTimeoutMs);
  if (first) return info(first, false, await readJson(stateFile));

  await mkdir(runtimeDir, { recursive: true, mode: 0o700 });
  const deadline = Date.now() + startupTimeoutMs;
  const owner = { pid: process.pid, nonce: randomUUID(), rootId, url: base.origin, createdAt: new Date().toISOString() };
  let locked = false;
  while (!locked && Date.now() < deadline) {
    locked = await acquire(lockFile, owner);
    if (locked) break;
    const health = await inspect(base, rootId, Math.min(probeTimeoutMs, Math.max(50, deadline - Date.now())));
    if (health) return info(health, false, await readJson(stateFile));
    await reapDeadLock(lockFile, rootId);
    await delay(Math.min(pollIntervalMs, Math.max(0, deadline - Date.now())));
  }
  if (!locked) throw new LocalServiceError('STARTUP_TIMEOUT', '另一启动请求仍在处理，等待本地画布就绪超时；没有重复启动。', { logFile });

  let child;
  let state;
  try {
    const health = await inspect(base, rootId, Math.min(probeTimeoutMs, Math.max(50, deadline - Date.now())));
    if (health) return info(health, false, await readJson(stateFile));
    // If a previous MCP process died after spawning Next, its child can still be
    // compiling. Wait for that owned instance; do not create a second child.
    const previous = await readJson(stateFile);
    if (previous?.rootId === rootId && typeof previous.instanceId === 'string' && alive(previous.launcherPid)) {
      while (Date.now() < deadline) {
        const ready = await inspect(base, rootId, Math.min(probeTimeoutMs, Math.max(50, deadline - Date.now())));
        if (ready) return info(ready, false, previous);
        if (!alive(previous.launcherPid)) break;
        await delay(pollIntervalMs);
      }
      if (alive(previous.launcherPid)) throw new LocalServiceError('STARTUP_TIMEOUT', '已有本项目启动进程仍在运行但尚未就绪；未停止或重复启动它。', { logFile, launcherPid: previous.launcherPid });
    }
    if (Date.now() >= deadline) throw new LocalServiceError('STARTUP_TIMEOUT', '等待本地画布启动锁超时；没有启动新进程。', { logFile });
    if (!await portIsFree(port)) throw new LocalServiceError('PORT_IN_USE', `端口 ${port} 已占用，且没有可复用的 Canvepp 健康实例；未启动新进程。`, { logFile });
    const next = path.join(root, 'node_modules', 'next', 'dist', 'bin', 'next');
    const launch = options.launch ?? { command: process.execPath, args: [next, 'dev', '--hostname', '127.0.0.1', '--port', String(port)] };
    if (!options.launch && !existsSync(next)) throw new LocalServiceError('DEPENDENCIES_MISSING', 'Canvepp 运行依赖尚未安装。请先完成一次 npm ci，启动器不会自动安装依赖。', { logFile });
    if (typeof launch.command !== 'string' || !path.isAbsolute(launch.command) || !Array.isArray(launch.args) || launch.args.some(arg => typeof arg !== 'string')) throw new LocalServiceError('INVALID_OPTIONS', '启动命令必须是绝对可执行路径和参数数组。');
    const instanceId = randomUUID();
    const cwd = launch.cwd ? await realpath(launch.cwd) : root;
    if (cwd !== root) throw new LocalServiceError('INVALID_OPTIONS', '启动进程的工作目录必须是当前 Canvepp 项目。');
    const log = openSync(logFile, 'a', 0o600);
    let spawnError;
    let exit;
    try {
      child = spawn(launch.command, launch.args, { cwd, env: { ...process.env, ...launch.env, CANVEPP_INSTANCE_ID: instanceId }, detached: process.platform !== 'win32', stdio: ['ignore', log, log], windowsHide: true, shell: false });
      child.once('error', error => { spawnError = error; });
      child.once('exit', (code, signal) => { exit = { code, signal }; });
      state = { service: 'canvepp', rootId, instanceId, launcherPid: child.pid, url: base.origin, startedAt: new Date().toISOString(), command: { file: launch.command, args: launch.args } };
      // Synchronous write closes the handoff gap before yielding to another MCP.
      atomicState(stateFile, state);
      appendFileSync(logFile, `\n[${state.startedAt}] Canvepp launcher pid=${child.pid ?? 'unavailable'} port=${port} rootId=${rootId}\n`);
      child.unref();
    } finally { closeSync(log); }
    while (Date.now() < deadline) {
      if (spawnError || exit) throw new LocalServiceError('STARTUP_FAILED', '本地画布启动进程提前退出，请查看启动日志；未重试或生成图片。', { logFile, launcherPid: child.pid, exitCode: exit?.code });
      const ready = await inspect(base, rootId, Math.min(probeTimeoutMs, Math.max(50, deadline - Date.now())));
      if (ready) {
        if (ready.instanceId !== instanceId) { await stopOwnedChild(child); return info(ready, false, await readJson(stateFile)); }
        state = { ...state, pid: ready.pid, readyAt: new Date().toISOString() };
        atomicState(stateFile, state);
        return info(ready, true, state);
      }
      await delay(Math.min(pollIntervalMs, Math.max(0, deadline - Date.now())));
    }
    throw new LocalServiceError('STARTUP_TIMEOUT', '本地画布未在限定时间内就绪，已停止本次启动的进程；没有提交任何生成。', { logFile, launcherPid: child.pid });
  } catch (error) {
    await stopOwnedChild(child);
    if (child && state) {
      const current = await readJson(stateFile).catch(() => undefined);
      if (current?.instanceId === state.instanceId) await unlink(stateFile).catch(() => {});
    }
    if (error instanceof LocalServiceError) throw error;
    throw new LocalServiceError('STARTUP_FAILED', '无法启动本地画布，请检查项目依赖与运行目录。', { logFile });
  } finally { await release(lockFile, owner.nonce); }
}
