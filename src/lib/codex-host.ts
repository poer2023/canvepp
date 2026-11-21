/** The short-lived capability opens a canvas whose chat association is durable. */
const HOST_STORAGE_KEY = 'canvepp:codex-host';
let memoryToken:string|undefined;
export interface CodexHostContext { connected:boolean;threadId?:string;label?:string;projectId?:string; }
export interface CodexDispatchReceipt { status:'queued'|'already_queued'|'unknown';threadId?:string;message?:string; }
export function consumeCodexHostToken():string|undefined {
    if(typeof window==='undefined')return undefined;
    const current=new URL(window.location.href);
    const params=new URLSearchParams(current.hash.slice(1));
    const token=params.get('canveppHost');
    if(token){
        // Remove the capability from the address bar before doing any work.
        params.delete('canveppHost');
        current.hash=params.toString();
        window.history.replaceState(window.history.state,'',current.pathname+current.search+current.hash);
        if(/^[a-f0-9]{64}$/i.test(token)){
            memoryToken=token;
            try{window.sessionStorage.setItem(HOST_STORAGE_KEY,token);}catch{/* Storage can be disabled; retain this tab's binding in memory. */}
        }
    }
    return getCodexHostToken();
}
export function getCodexHostToken():string|undefined {
    if(typeof window==='undefined')return undefined;
    if(memoryToken)return memoryToken;
    try{const stored=window.sessionStorage.getItem(HOST_STORAGE_KEY);return stored&&/^[a-f0-9]{64}$/i.test(stored)?stored:undefined;}catch{return undefined;}
}
export async function readCodexHostContext():Promise<CodexHostContext> {
    const token=consumeCodexHostToken();
    if(!token)return {connected:false};
    const response=await fetch('/api/host/context',{headers:{'X-Canvepp-Host':token},cache:'no-store'});
    if(!response.ok)return {connected:false};
    const context=await response.json() as CodexHostContext;
    return {connected:context.connected===true,...(typeof context.threadId==='string'?{threadId:context.threadId}:{}),...(typeof context.label==='string'?{label:context.label}:{}),...(typeof context.projectId==='string'?{projectId:context.projectId}:{})};
}
