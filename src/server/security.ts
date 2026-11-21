import { randomBytes } from 'node:crypto';
import type { CanvasStore } from './store';
import { AppError, assert } from './errors';
const COOKIE='canvepp_session';
const localHost=(hostname:string)=>['127.0.0.1','localhost','[::1]','::1'].includes(hostname);
export function checkOrigin(request:Request){const internal=new URL(request.url);assert(localHost(internal.hostname),'此服务仅允许本机访问',403,'authorization');const host=request.headers.get('host')??internal.host;let external:URL;try{external=new URL(`${internal.protocol}//${host}`);}catch{throw new AppError('主机校验失败',403,'authorization');}assert(localHost(external.hostname)&&external.host===host&&!external.username&&!external.password&&external.pathname==='/'&&!external.search&&!external.hash,'主机校验失败',403,'authorization');const origin=request.headers.get('origin');assert(!origin||origin===external.origin,'来源校验失败',403,'authorization');assert(!['cross-site','none'].includes(request.headers.get('sec-fetch-site')??''),'不允许跨站请求',403,'authorization');}
function cookieName(request?: Request) {
 if (!request) return COOKIE;
 const url = new URL(request.url);
 const {port} = new URL(url.protocol + '//' + (request.headers.get('host') || url.host));
 return port && port !== '3210' ? COOKIE + '_' + port : COOKIE;
}
function cookie(request:Request) {
 const entries = (request.headers.get('cookie') || '').split(';').map(value=>value.trim());
 const read = (key:string) => entries.find(value=>value.startsWith(key+'='))?.slice(key.length+1);
 return read(cookieName(request)) ?? read(COOKIE);
}
export function session(store:CanvasStore,request:Request,create=false){checkOrigin(request);const token=cookie(request);const row=token?store.db.prepare('SELECT csrf,expires_at FROM sessions WHERE token=?').get(token) as {csrf:string;expires_at:number}|undefined:undefined;if(row&&row.expires_at>Date.now())return {token:token!,csrf:row.csrf,newSession:false};assert(create,'本地会话已失效，请刷新页面',401,'session_expired');const newToken=randomBytes(32).toString('hex'),csrf=randomBytes(32).toString('hex');store.db.prepare('DELETE FROM sessions WHERE expires_at<?').run(Date.now());store.db.prepare('INSERT INTO sessions VALUES (?,?,?)').run(newToken,csrf,Date.now()+7*24*60*60_000);return {token:newToken,csrf,newSession:true};}
export function authorize(store:CanvasStore,request:Request){const s=session(store,request);if(!['GET','HEAD'].includes(request.method))assert(request.headers.get('x-csrf-token')===s.csrf,'本地会话确认失败，请刷新页面',403,'csrf_mismatch');return s;}
export function sessionCookie(token:string,request?:Request){return `${cookieName(request)}=${token}; Path=/; HttpOnly; SameSite=Strict; Max-Age=604800`;}
