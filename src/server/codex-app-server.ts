import { spawn } from 'node:child_process';
import { createInterface } from 'node:readline';
import { codexBinary } from './host-dispatch';

export interface NativeItem {
    type: string;
    id: string;
    status?: string;
    savedPath?: string;
    result?: string;
    content?: { type: string; text?: string }[];
    failure?: { message?: string } | null;
}
export interface NativeTurn {
    id: string;
    status: string;
    items: NativeItem[];
    error?: { message: string } | null;
}
export type NativeNotice = { threadId?: string; turnId?: string; item?: NativeItem; turn?: NativeTurn };
interface PendingCall {
    resolve: (value: unknown) => void;
    reject: (error: Error) => void;
    timer: ReturnType<typeof setTimeout>;
}

// A local stdio client. Only Codex owns the signed-in account and model transport.
export class CodexAppServer {
    private child = spawn(/*turbopackIgnore: true*/ codexBinary(), ['app-server', '--stdio'], {
        shell: false, windowsHide: true, stdio: ['pipe', 'pipe', 'ignore'],
    });
    private calls = new Map<number, PendingCall>();
    private seq = 0;
    private ended = false;
    onNotice?: (method: string, params: NativeNotice) => void;
    onDisconnect?: () => void;

    constructor() {
        const end = () => {
            if (this.ended) return;
            this.ended = true;
            for (const call of this.calls.values()) {
                clearTimeout(call.timer);
                call.reject(new Error('本机 Codex 连接已断开'));
            }
            this.calls.clear();
            this.onDisconnect?.();
        };
        this.child.on('error', end);
        this.child.on('close', end);
        this.child.stdin.on('error', end);
        createInterface({input: this.child.stdout}).on('line', line => {
            if (this.ended) return;
            let msg;
            try { msg = JSON.parse(line); } catch { return; }
            if (typeof msg.method === 'string') {
                if (msg.id !== undefined) {
                    // A canvas generation grants only the built-in image operation.
                    this.write({ id: msg.id, error: { code: -32601, message: 'This canvas cannot approve external tools.' } });
                } else {
                    this.onNotice?.(msg.method, msg.params ?? {});
                }
                return;
            }
            const call = this.calls.get(msg.id);
            if (!call) return;
            this.calls.delete(msg.id);
            clearTimeout(call.timer);
            if (msg.error) call.reject(new Error(String(msg.error.message ?? 'Codex 请求失败').slice(0, 280)));
            else call.resolve(msg.result);
        });
    }
    private write(message: unknown) {
        if (this.ended || !this.child.stdin.writable) throw new Error('本机 Codex 未连接');
        this.child.stdin.write(JSON.stringify(message) + '\n');
    }
    call<T>(method: string, params: unknown): Promise<T> {
        const id = ++this.seq;
        return new Promise<T>((resolve, reject) => {
            const timer = setTimeout(() => { this.calls.delete(id); reject(new Error('本机 Codex 请求超时')); }, 35_000);
            this.calls.set(id, { resolve: value => resolve(value as T), reject, timer });
            try { this.write({ id, method, params }); }
            catch (error) { clearTimeout(timer); this.calls.delete(id); reject(error); }
        });
    }
    async initialize() {
        await this.call('initialize', { clientInfo: { name: 'canvepp', title: 'Canvepp', version: '1' }, capabilities: {} });
        this.write({ method: 'initialized' });
    }
    async requireNativeAccount() {
        const account = await this.call<{ account: { type: string } | null }>('account/read', {});
        if (account.account?.type !== 'chatgpt') throw new Error('请先用 ChatGPT 账号登录本机 Codex，然后回到画布生成。');
        const capabilities = await this.call<{ imageGeneration: boolean }>('modelProvider/capabilities/read', {});
        if (!capabilities.imageGeneration) throw new Error('当前 Codex 登录或所选提供方未开放原生生图。');
    }
    close() {
        this.onDisconnect = undefined;
        this.onNotice = undefined;
        this.child.stdin.end();
        this.child.kill();
    }
}
