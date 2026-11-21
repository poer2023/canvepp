#!/usr/bin/env node
import fs from 'node:fs/promises';
import path from 'node:path';
import os from 'node:os';
import {randomUUID} from 'node:crypto';
import {pathToFileURL} from 'node:url';
import {parseArgs} from 'node:util';
import {ensureLocalService} from './local-service.mjs';

const UUID=/^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/i;
export function createCanvasClient(options={}){
 const root=path.resolve(options.root||path.join(import.meta.dirname,'..'));
 const base=new URL(options.baseUrl||process.env.CANVEPP_URL||'http://127.0.0.1:3210');
 if(base.protocol!=='http:'||base.hostname!=='127.0.0.1'||base.username||base['pass'+'word']||base.pathname!=='/'||base.search||base.hash)throw Error('Canvepp only accepts a loopback origin.');
 const receipts=path.resolve(options.receiptDir||path.join(root,'.data','runtime','agent'));
 let credentials;
 async function session(){const r=await fetch(new URL('/api/session',base),{headers:{Origin:base.origin},redirect:'error',signal:AbortSignal.timeout(10000)});if(!r.ok)throw Error(`本地会话失败 (${r.status})`);const data=await r.json();credentials={cookie:r.headers.get('set-cookie')?.split(';')[0],csrf:data.csrfToken};if(!credentials.cookie||!credentials.csrf)throw Error('服务未返回有效会话');return credentials;}
 async function request(route,method='GET',body,auth){const c=auth||credentials||await session();const headers={Origin:base.origin,Cookie:c.cookie};let payload;if(method!=='GET'){headers['X-CSRF-Token']=c.csrf;if(body instanceof FormData){if(!body.has('requestId'))body.set('requestId',randomUUID());payload=body;}else{headers['Content-Type']='application/json';payload=JSON.stringify({...body,requestId:randomUUID()});}}
  const r=await fetch(new URL('/api'+route,base),{method,headers,body:payload,redirect:'error',signal:AbortSignal.timeout(60000)});if(!r.ok){let message=`请求失败 (${r.status})`;try{message=(await r.json()).error||message;}catch{}throw Error(message);}return r;
 }
 async function json(route,method='GET',body,auth){return (await request(route,method,body,auth)).json();}
 function receiptFile(runId){if(!UUID.test(runId))throw Error('无效 runId');return path.join(receipts,runId+'.json');}
 async function saveReceipt(runId,value){await fs.mkdir(receipts,{recursive:true,mode:0o700});const file=receiptFile(runId),temp=file+'.'+randomUUID()+'.tmp';await fs.writeFile(temp,JSON.stringify(value),{flag:'wx',mode:0o600});await fs.rename(temp,file);}
 async function readReceipt(runId){try{const data=JSON.parse(await fs.readFile(receiptFile(runId),'utf8'));if(data.origin!==base.origin)throw Error('回填服务与领取服务不一致');return data;}catch(error){if(error.code==='ENOENT')return undefined;throw error;}}
 async function open(threadId=process.env.CODEX_THREAD_ID,projectId){
  if(threadId&&!UUID.test(threadId))throw Error('无效 Codex 对话标识');
  if(projectId&&!UUID.test(projectId))throw Error('无效画布标识');
  const service=await ensureLocalService({baseUrl:base.origin,projectRoot:root});
  const linked=projectId || (threadId ? (await json('/host/linked-canvas?threadId='+encodeURIComponent(threadId))).projectId : undefined);
  const url=new URL('/',base);
  if(linked)url.searchParams.set('canvas',linked);
  return {url:url.href,available:true,projectId:linked,service,mode:'codex-native',generationSubmitted:false};
 }
 async function claim(runId,threadId=process.env.CODEX_THREAD_ID){
  const lock=receiptFile(runId)+'.lock';await fs.mkdir(receipts,{recursive:true,mode:0o700});
  let handle;try{handle=await fs.open(lock,'wx',0o600);}catch(error){if(error.code==='EEXIST')throw Error('此任务的领取正在执行或曾中断。不要并发领取或重新生成，请先检查任务状态。');throw error;}
  try{return await claimUnlocked(runId,threadId);}finally{await handle.close();await fs.unlink(lock).catch(()=>{});}
 }
 async function claimUnlocked(runId,threadId){const prior=await readReceipt(runId);if(prior?.claimed)return {run:await json(`/runs/${runId}`,'GET',undefined,prior.credentials),canGenerate:false,alreadyClaimed:true,sourcePath:prior.sourcePath,sourcePaths:prior.sourcePaths,message:'此脚本已领取过任务；不得再次调用 imagegen。已有结果用 complete 回填，结果未知用 fail unknown。'};
  if(threadId&&!UUID.test(threadId))throw Error('无效 Codex 会话标识');
  const auth=prior?.credentials||await session();const claimId=prior?.claimId||randomUUID(),clientId=prior?.clientId||randomUUID();
  // Persist intent before transport: an ambiguous response must never produce a fresh claim ID.
  await saveReceipt(runId,{origin:base.origin,credentials:auth,claimId,clientId,claimed:false});
  const data=await json(`/runs/${runId}/host/claim`,'POST',{claimId,clientId,...(threadId?{threadId}:{})},auth);
  // Retain completion/failure authority even if downloading the reference fails.
  await saveReceipt(runId,{origin:base.origin,credentials:auth,claimId,clientId,claimToken:data.claimToken,claimed:true});
  const sourcePaths=[];
  const sourceIds=data.run.inputSnapshot.sourceAssetVersionIds||(data.run.inputSnapshot.sourceAssetVersionId?[data.run.inputSnapshot.sourceAssetVersionId]:[]);
  for(let index=0;index<sourceIds.length;index++){const response=await request(`/assets/${sourceIds[index]}`,'GET',undefined,auth);const bytes=Buffer.from(await response.arrayBuffer());if(bytes.length>20*1024*1024)throw Error('参考图超出大小限制');const mime=response.headers.get('content-type');const ext=mime==='image/jpeg'?'jpg':mime==='image/webp'?'webp':'png';await fs.mkdir(path.join(receipts,'inputs'),{recursive:true,mode:0o700});const sourcePath=path.join(receipts,'inputs',`${runId}${sourceIds.length>1?`-ref-${index+1}`:''}.${ext}`);await fs.writeFile(sourcePath,bytes,{mode:0o600});sourcePaths.push(sourcePath);}
  const sourcePath=sourcePaths[0];
  await saveReceipt(runId,{origin:base.origin,credentials:auth,claimId,clientId,claimToken:data.claimToken,claimed:true,sourcePath,sourcePaths});
  return {run:data.run,canGenerate:data.canGenerate,alreadyClaimed:data.alreadyClaimed,sourcePath,sourcePaths,instructions:'仅 canGenerate=true 时使用内置 image_gen 一次，一次一张。不要用独立 API/Magpie。读取不可变提示词和目标宽高像素；sourcePaths 按顺序列出所有固定参考图，逐张 view_image 检查，并将完整 sourcePaths 作为 referenced_image_paths 一并交给 image_gen，不能只传第一张。sourcePath 是兼容旧单图的第一张路径。完成后用同一 runId 回填；不要自行采用版本或创建卡片。仅已确认的新结果卡片由服务器安全填入首次结果，原图不变。'};
 }
 async function complete(runId,file){const record=await readReceipt(runId);if(!record?.claimToken)throw Error('找不到此任务的本地领取凭证，不能猜测或重新生图');
  const canonical=await fs.realpath(path.resolve(file));const allowed=[await fs.realpath(root),path.resolve(process.env.CODEX_HOME||path.join(os.homedir(),'.codex'),'generated_images')];
  if(!allowed.some(dir=>canonical.startsWith(dir+path.sep)))throw Error('结果须位于本项目或 Codex generated_images 目录');
  const stat=await fs.stat(canonical);if(!stat.isFile()||stat.size>20*1024*1024||stat.size===0)throw Error('结果图片无效或超过20MB');const bytes=await fs.readFile(canonical);const form=new FormData();form.set('claimToken',record.claimToken);form.set('file',new Blob([bytes]),path.basename(canonical));const result=await json(`/runs/${runId}/host/complete`,'POST',form,record.credentials);return {run:result,generationSubmitted:false};
 }
 async function fail(runId,outcome='unknown',message='Codex 原生工具执行未完成，请检查当前对话。'){const record=await readReceipt(runId);if(!record?.claimToken)throw Error('缺少任务领取凭证');if(!['failed','unknown'].includes(outcome))throw Error('outcome must be failed or unknown');return json(`/runs/${runId}/host/fail`,'POST',{claimToken:record.claimToken,outcome,message},record.credentials);}
 return {root,base,open,claim,complete,fail,json};
}
async function main(){const command=process.argv[2];const {values}=parseArgs({args:process.argv.slice(3),options:{thread:{type:'string'},project:{type:'string'},run:{type:'string'},file:{type:'string'},outcome:{type:'string'},message:{type:'string'}}});const client=createCanvasClient();let result;
 if(command==='open')result=await client.open(values.thread,values.project);
 else if(command==='claim'&&values.run)result=await client.claim(values.run,values.thread);
 else if(command==='complete'&&values.run&&values.file)result=await client.complete(values.run,values.file);
 else if(command==='fail'&&values.run)result=await client.fail(values.run,values.outcome,values.message);
 else if(command==='status'&&values.run)result=await client.json(`/runs/${values.run}`);
 else throw Error('Usage: canvas-agent.mjs open [--thread UUID] [--project UUID] | claim --run UUID [--thread UUID] | complete --run UUID --file ABSOLUTE_PATH | fail --run UUID --outcome unknown|failed | status --run UUID');
 console.log(JSON.stringify(result));
}
if(process.argv[1]&&import.meta.url===pathToFileURL(path.resolve(process.argv[1])).href)main().catch(error=>{console.error(JSON.stringify({error:error.message,generationSubmitted:false}));process.exitCode=1;});
