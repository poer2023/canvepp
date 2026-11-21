import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { createServer, type Server, type IncomingMessage } from 'node:http';
import { mkdtempSync, mkdirSync, readFileSync, rmSync, statSync, writeFileSync } from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { randomUUID } from 'node:crypto';
import { createCanvasClient } from '../scripts/canvas-agent.mjs';

let dir:string,root:string,receipts:string,server:Server|undefined;
const releases:(()=>void)[]=[];
beforeEach(()=>{dir=mkdtempSync(path.join(os.tmpdir(),'canvepp-agent-http-'));root=path.join(dir,'project');receipts=path.join(dir,'private-receipts');mkdirSync(root);});
afterEach(async()=>{for(const release of releases.splice(0))release();if(server){server.closeAllConnections();await new Promise<void>(resolve=>server!.close(()=>resolve()));server=undefined;}rmSync(dir,{recursive:true,force:true});});
function gate(){let release!:()=>void;const promise=new Promise<void>(resolve=>{release=resolve;});releases.push(release);return {promise,release};}
async function body(request:IncomingMessage){const chunks:Buffer[]=[];for await(const chunk of request)chunks.push(Buffer.from(chunk));return Buffer.concat(chunks);}
async function fixture(options:{sourceFailure?:boolean;sourceIds?:string[];blockClaim?:Promise<void>}={}){
 const runId=randomUUID(),claimToken='a'.repeat(64),claimed=gate();
 const run={id:runId,mode:'codex',status:'running',inputSnapshot:{prompt:'本地测试任务',params:{size:'713x509',quality:'high',n:1},...(options.sourceFailure?{sourceAssetVersionId:'fixture-source'}:{}),...(options.sourceIds?{sourceAssetVersionId:options.sourceIds[0],sourceAssetVersionIds:options.sourceIds}:{})}};
 const state={sessions:0,claims:0,reads:0,sourceReads:0,failures:0,uploads:[] as {claimToken:string;requestId:string;name:string;bytes:Buffer}[],claimBodies:[] as Record<string,unknown>[],errors:[] as unknown[]};
 const sessions=new Map<string,string>();let origin='';
 server=createServer(async(request,response)=>{
  const send=(value:unknown,status=200)=>{response.writeHead(status,{'Content-Type':'application/json'});response.end(JSON.stringify(value));};
  try{
   expect(request.headers.origin).toBe(origin);
   if(request.url==='/api/session'&&request.method==='GET'){state.sessions++;const cookie=`canvepp_session=fixture-${state.sessions}`,csrf=`csrf-${state.sessions}`;sessions.set(cookie,csrf);response.setHeader('Set-Cookie',`${cookie}; HttpOnly; SameSite=Strict`);send({csrfToken:csrf});return;}
   const csrf=sessions.get(String(request.headers.cookie));expect(csrf).toBeTruthy();if(request.method!=='GET')expect(request.headers['x-csrf-token']).toBe(csrf);
   if(request.url===`/api/runs/${runId}/host/claim`){state.claims++;state.claimBodies.push(JSON.parse((await body(request)).toString()));claimed.release();if(options.blockClaim)await options.blockClaim;send({run,claimToken,canGenerate:true,alreadyClaimed:false});return;}
   if(request.url===`/api/runs/${runId}`&&request.method==='GET'){state.reads++;send(run);return;}
   if(request.url==='/api/assets/fixture-source'){state.sourceReads++;send({error:'fixture source unavailable'},503);return;}
   if(options.sourceIds?.some(id=>request.url===`/api/assets/${id}`)){state.sourceReads++;response.writeHead(200,{'Content-Type':'image/png'});response.end(Buffer.from(request.url!.split('/').at(-1)!));return;}
   if(request.url===`/api/runs/${runId}/host/fail`){const data=JSON.parse((await body(request)).toString());expect(data.claimToken).toBe(claimToken);expect(data.outcome).toBe('failed');expect(data.requestId).toBeTruthy();state.failures++;send({...run,status:'failed'});return;}
   if(request.url===`/api/runs/${runId}/host/complete`){const bytes=await body(request);const form=await new Request(origin+request.url,{method:'POST',headers:{'content-type':String(request.headers['content-type'])},body:bytes}).formData();const file=form.get('file');expect(file).toBeInstanceOf(File);expect(form.getAll('file')).toHaveLength(1);state.uploads.push({claimToken:String(form.get('claimToken')),requestId:String(form.get('requestId')),name:(file as File).name,bytes:Buffer.from(await (file as File).arrayBuffer())});send({...run,status:'succeeded',assetVersionIds:['fixture-result']});return;}
   throw Error(`Unexpected fixture route ${request.method} ${request.url}`);
  }catch(error){state.errors.push(error);send({error:'HTTP fixture assertion failed'},500);}
 });
 await new Promise<void>(resolve=>server!.listen(0,'127.0.0.1',resolve));const address=server.address();if(!address||typeof address==='string')throw Error('missing fixture port');origin=`http://127.0.0.1:${address.port}`;
 const client=()=>createCanvasClient({root,receiptDir:receipts,baseUrl:origin});
 const receipt=()=>JSON.parse(readFileSync(path.join(receipts,`${runId}.json`),'utf8'));
 return {runId,claimToken,run,state,origin,client,receipt,claimed:claimed.promise};
}

describe('canvas-agent HTTP client receipts',()=>{
 it('downloads every frozen reference in order and keeps all paths without granting a second generation',async()=>{
  const f=await fixture({sourceIds:['second-picture','first-picture','third-picture']});
  const claimed=await f.client().claim(f.runId);expect(claimed.canGenerate).toBe(true);expect(claimed.sourcePaths).toHaveLength(3);
  expect(claimed.sourcePaths.map((file:string)=>readFileSync(file,'utf8'))).toEqual(['second-picture','first-picture','third-picture']);
  expect(claimed.sourcePath).toBe(claimed.sourcePaths[0]);expect(f.receipt().sourcePaths).toEqual(claimed.sourcePaths);
  const replay=await f.client().claim(f.runId);expect(replay.canGenerate).toBe(false);expect(replay.sourcePaths).toEqual(claimed.sourcePaths);expect(f.state.claims).toBe(1);expect(f.state.sourceReads).toBe(3);expect(f.state.errors).toEqual([]);
 });
 it('allows only one concurrent claim HTTP call across separate clients and preserves its private receipt',async()=>{
  const responseGate=gate(),f=await fixture({blockClaim:responseGate.promise});const first=f.client().claim(f.runId);await f.claimed;
  const intent=f.receipt();expect(intent.claimed).toBe(false);expect(intent.credentials.cookie).toBe('canvepp_session=fixture-1');
  await expect(f.client().claim(f.runId)).rejects.toThrow('领取正在执行');expect(f.state.claims).toBe(1);expect(f.state.sessions).toBe(1);
  responseGate.release();const result=await first;expect(result.canGenerate).toBe(true);expect(result.run.inputSnapshot.params).toEqual({size:'713x509',quality:'high',n:1});const saved=f.receipt();expect(saved.claimToken).toBe(f.claimToken);expect(saved.claimId).toBe(intent.claimId);expect(saved.clientId).toBe(intent.clientId);expect(saved.credentials).toEqual(intent.credentials);expect(saved.claimed).toBe(true);expect(f.state.claimBodies[0]).toMatchObject({claimId:saved.claimId,clientId:saved.clientId});expect(statSync(path.join(receipts,`${f.runId}.json`)).mode&0o777).toBe(0o600);expect(JSON.stringify(result)).not.toContain(f.claimToken);expect(f.state.errors).toEqual([]);
 });
 it('reuses a persisted claimed receipt after a new client starts and refuses a second generation',async()=>{
  const f=await fixture();expect((await f.client().claim(f.runId)).canGenerate).toBe(true);const original=f.receipt();const replay=await f.client().claim(f.runId);expect(replay.canGenerate).toBe(false);expect(replay.alreadyClaimed).toBe(true);expect(replay.run.inputSnapshot.params.size).toBe('713x509');expect(f.state.claims).toBe(1);expect(f.state.sessions).toBe(1);expect(f.state.reads).toBe(1);expect(f.receipt()).toEqual(original);expect(JSON.stringify(replay)).not.toContain(f.claimToken);expect(f.state.errors).toEqual([]);
 });
 it('retains the returned token before a reference-image failure so the host can report failure without claiming again',async()=>{
  const f=await fixture({sourceFailure:true});await expect(f.client().claim(f.runId)).rejects.toThrow('fixture source unavailable');expect(f.receipt()).toMatchObject({claimToken:f.claimToken,claimed:true});const failed=await f.client().fail(f.runId,'failed','参考图下载失败，尚未调用生图');expect(failed.status).toBe('failed');expect(f.state.failures).toBe(1);expect((await f.client().claim(f.runId)).canGenerate).toBe(false);expect(f.state.claims).toBe(1);expect(f.state.sourceReads).toBe(1);expect(f.state.errors).toEqual([]);
 });
 it('uploads exactly the selected local image and saved claim in real multipart HTTP',async()=>{
  const f=await fixture();await f.client().claim(f.runId);const bytes=Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aS1sAAAAASUVORK5CYII=','base64');const file=path.join(root,'actual-result.png');writeFileSync(file,bytes);const result=await f.client().complete(f.runId,file);expect(result.run.status).toBe('succeeded');expect(result.generationSubmitted).toBe(false);expect(f.state.uploads).toHaveLength(1);expect(f.state.uploads[0]).toMatchObject({claimToken:f.claimToken,name:'actual-result.png',bytes});expect(f.state.uploads[0].requestId).toMatch(/^[a-f0-9-]{36}$/);expect(f.state.claims).toBe(1);expect(f.state.sessions).toBe(1);expect(f.state.errors).toEqual([]);
 });
});
