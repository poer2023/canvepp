import { createHash, randomUUID } from 'node:crypto';
import { realpathSync } from 'node:fs';
import type { Graph, BudgetSettings, RunMode, RunPreview } from '@/domain/types';
import { getStore } from '@/server/store';
import { startCanvasExecutions, checkCanvasResult, isCanvasExecution } from '@/server/canvas-execution';
import { deleteProject } from '@/server/project-management';
import { AppError, assert, publicError } from '@/server/errors';
import { authorize, checkOrigin, session, sessionCookie } from '@/server/security';
import { requireCanvasContext } from '@/server/host-dispatch';
import { assertCanvasThread, canvasThread, threadCanvas } from '@/server/thread-canvas';
import { listTrash, restoreTrashedCards } from '@/server/trash';
import { searchLibrary } from '@/server/image-library';
import { confirmGenerationGroup, getGenerationPreview, previewGenerationGroup } from '@/server/generation-groups';
import type { ImageParams } from '@/domain/types';
export const runtime='nodejs';
export const dynamic='force-dynamic';
const processState=globalThis as unknown as {canveppInstanceId?:string};
const instanceId=processState.canveppInstanceId??=process.env.CANVEPP_INSTANCE_ID||randomUUID();
const baseHeaders={'Cache-Control':'no-store','X-Content-Type-Options':'nosniff','Cross-Origin-Resource-Policy':'same-origin','Referrer-Policy':'no-referrer'};
function response(body:unknown,status=200,extra:Record<string,string>={}) {return Response.json(body,{status,headers:{...baseHeaders,...extra}});}
async function readBody(request:Request,maxBytes:number):Promise<Uint8Array<ArrayBuffer>>{assert(Number(request.headers.get('content-length')??0)<=maxBytes,'请求内容过大',413);const reader=request.body?.getReader();if(!reader)return new Uint8Array();const chunks:Uint8Array[]=[];let total=0;while(true){const {done,value}=await reader.read();if(done)break;total+=value.byteLength;if(total>maxBytes){await reader.cancel();throw new AppError('请求内容过大',413);}chunks.push(value);}const out=new Uint8Array(total);let offset=0;for(const chunk of chunks){out.set(chunk,offset);offset+=chunk.byteLength;}return out;}
async function jsonBody(request:Request):Promise<Record<string,unknown>>{const bytes=await readBody(request,2*1024*1024);let body:unknown;try{body=JSON.parse(new TextDecoder().decode(bytes));}catch{throw new AppError('JSON 格式无效');}assert(body&&typeof body==='object'&&!Array.isArray(body),'请求格式无效');const record=body as Record<string,unknown>;assert(typeof record.requestId==='string'&&record.requestId.length>0&&record.requestId.length<=160,'缺少 requestId');return record;}
async function formBody(request:Request){const bytes=await readBody(request,51*1024*1024);let form:FormData;try{form=await new Request(request.url,{method:'POST',headers:{'content-type':request.headers.get('content-type')??''},body:bytes}).formData();}catch{throw new AppError('文件上传格式无效，请使用 multipart/form-data。');}assert(typeof form.get('requestId')==='string'&&form.get('requestId'),'缺少 requestId');return form;}
const string=(v:unknown,name:string)=>{assert(typeof v==='string'&&v.length>0,`缺少 ${name}`);return v;};
async function handler(request:Request,context:{params:Promise<{path?:string[]}>}){
 try{
  checkOrigin(request);const parts=(await context.params).path??[];const route=parts.join('/'),method=request.method;
  if(route==='health'&&method==='GET')return response({service:'canvepp',protocolVersion:1,rootId:createHash('sha256').update(realpathSync(process.cwd())).digest('hex'),instanceId,pid:process.pid});
  const store=getStore();
  if(route==='session'&&method==='GET'){const s=session(store,request,true);return response({csrfToken:s.csrf},200,{'Set-Cookie':sessionCookie(s.token,request)});}
  const authorized=authorize(store,request);
  if(route==='host/linked-canvas'&&method==='GET'){
   const threadId=new URL(request.url).searchParams.get('threadId')??'';
   assert(/^[a-f0-9]{8}(?:-[a-f0-9]{4}){3}-[a-f0-9]{12}$/i.test(threadId),'会话标识无效');
   const associated=store.db.prepare('SELECT project_id FROM canvas_native_jobs WHERE thread_id=? LIMIT 1').get(threadId) as {project_id:string}|undefined;
   return response({projectId:associated?.project_id??threadCanvas(store,threadId)?.projectId??null});
  }
  if(route==='image-library'&&method==='GET')return response(searchLibrary(store,new URL(request.url).searchParams));
  if(route==='generation-groups/preview'&&method==='POST'){const b=await jsonBody(request);const projectId=string(b.projectId,'projectId');if(b.mode==='codex' && b.execution!=='canvas')requireCanvasContext(store,request.headers.get('x-canvepp-host')??undefined,projectId);return response(previewGenerationGroup(store,{projectId,...(b.execution==='canvas' && {execution:'canvas'}),graphRevision:b.graphRevision as number,mode:b.mode as RunMode,sourceNodeIds:b.sourceNodeIds as string[],prompt:b.prompt as string|undefined,params:b.params as ImageParams|undefined}));}
  if(route==='generation-groups'&&method==='POST'){const b=await jsonBody(request);const preview=getGenerationPreview(store,string(b.previewId,'previewId'));if(preview.mode==='codex' && preview.execution!=='canvas')requireCanvasContext(store,request.headers.get('x-canvepp-host')??undefined,preview.projectId);
   const result=confirmGenerationGroup(store,{previewId:preview.id,snapshotHash:string(b.snapshotHash,'snapshotHash'),confirmationKey:string(b.confirmationKey,'confirmationKey'),acceptStale:b.acceptStale===true});
   if(preview.execution==='canvas') startCanvasExecutions(store);
   return response(result,201);}
  if(parts[0]==='projects'&&parts[2]==='trash'&&parts.length===3&&method==='GET')return response({items:listTrash(store,parts[1])});
  if(parts[0]==='projects'&&parts[2]==='trash'&&parts[3]==='restore'&&parts.length===4&&method==='POST'){const b=await jsonBody(request);return response(restoreTrashedCards(store,parts[1],b.entryIds as string[],b.expectedRevision as number));}
  if(route==='projects'&&method==='GET')return response({projects:store.listProjects()});
  if(route==='projects'&&method==='POST'){const b=await jsonBody(request);assert(b.template===undefined||b.template==='image','不支持的项目模板');return response(await store.createProject(string(b.name,'name'),b.starter===true,b.template),201);}
  if(route==='projects/import'&&method==='POST'){const f=await formBody(request),file=f.get('file');assert(file instanceof File,'缺少项目文件');return response(await store.importProject(Buffer.from(await file.arrayBuffer())),201);}
  if(parts[0]==='projects'&&parts.length===2&&method==='DELETE'){const b=await jsonBody(request);return response(deleteProject(store,parts[1],{confirmed:b.confirmed===true||b.confirm===true,expectedRevision:b.expectedRevision as number}));}
  if(parts[0]==='projects'&&parts.length===2&&method==='GET')return response(store.detail(parts[1]));
  if(parts[0]==='projects'&&parts[2]==='graph'&&parts.length===3&&method==='PATCH'){const b=await jsonBody(request);assert(Number.isInteger(b.expectedRevision),'缺少 expectedRevision');return response(store.patchGraph(parts[1],b.expectedRevision as number,b.graph as Graph,typeof b.name==='string'?b.name:undefined));}
  if(parts[0]==='projects'&&parts[2]==='export'&&parts.length===3&&method==='GET'){const zip=await store.exportProject(parts[1]);return new Response(new Uint8Array(zip),{headers:{...baseHeaders,'Content-Type':'application/zip','Content-Disposition':'attachment; filename="canvepp-project.zip"'}});}
  if(parts[0]==='projects'&&parts[2]==='restore'&&parts.length===3&&method==='POST'){const f=await formBody(request),file=f.get('file');assert(file instanceof File,'缺少画布备份');const revision=f.get('expectedRevision');assert(typeof revision==='string'&&/^\d+$/.test(revision),'缺少画布版本');return response(await store.importProject(Buffer.from(await file.arrayBuffer()),{projectId:parts[1],expectedRevision:Number(revision)}));}
  if(parts[0]==='projects'&&parts[2]==='diagnostics'&&parts.length===3&&method==='GET')return response(store.diagnostics(parts[1]));
  if(route==='assets/import'&&method==='POST'){const f=await formBody(request),file=f.get('file');assert(file instanceof File,'缺少图片文件');const asset=await store.importAsset(Buffer.from(await file.arrayBuffer()),string(f.get('projectId'),'projectId'),file.name,typeof f.get('metadataPolicy')==='string'?String(f.get('metadataPolicy')):'strip',typeof f.get('nodeId')==='string'?String(f.get('nodeId')):undefined);return response(asset,201);}
  if(parts[0]==='assets'&&parts.length===2&&method==='GET'){const asset=store.getAsset(parts[1]);const thumbnail=new URL(request.url).searchParams.get('thumbnail')==='1';const bytes=thumbnail?await store.assetThumbnail(asset.id):store.assetBytes(asset.id);const mime=thumbnail?'image/webp':asset.mime;return new Response(new Uint8Array(bytes),{headers:{...baseHeaders,'Content-Type':mime,'Content-Length':String(bytes.length),'Content-Disposition':`inline; filename="image.${mime.split('/')[1]}"`}});}
  if(parts[0]==='assets'&&parts.length===2&&method==='PATCH'){const b=await jsonBody(request);return response(store.patchAsset(parts[1],{...(b.favorite!==undefined?{favorite:b.favorite as boolean}:{}),...(b.assessment!==undefined?{assessment:b.assessment as 'pass'|'continue'|'redo'}:{})}));}
  if(route==='runs/preview'&&method==='POST'){const b=await jsonBody(request);const projectId=string(b.projectId,'projectId');if(b.mode==='codex')requireCanvasContext(store,request.headers.get('x-canvepp-host')??undefined,projectId);return response(store.preview(projectId,string(b.nodeId,'nodeId'),b.graphRevision as number,b.mode as RunMode));}
  if(route==='runs'&&method==='POST'){const b=await jsonBody(request);const previewId=string(b.previewId,'previewId');const row=store.db.prepare('SELECT body FROM previews WHERE id=?').get(previewId) as {body:string}|undefined;if(row){const preview=JSON.parse(row.body) as RunPreview;if(preview.mode==='codex')requireCanvasContext(store,request.headers.get('x-canvepp-host')??undefined,preview.projectId);}return response(store.confirm({previewId,snapshotHash:string(b.snapshotHash,'snapshotHash'),confirmationId:string(b.confirmationId,'confirmationId'),idempotencyKey:string(b.idempotencyKey,'idempotencyKey'),acceptStale:b.acceptStale===true}),201);}
  if(route==='host/pending'&&method==='GET')return response({runs:store.pendingHostRuns(new URL(request.url).searchParams.get('projectId')??undefined)});
  if(parts[0]==='runs'&&parts[2]==='host'&&parts.length===4&&method==='POST'){
   if(parts[3]==='claim'){const b=await jsonBody(request);const run=store.getRun(parts[1]);assert(!isCanvasExecution(store,run.id),'该任务由画布执行器领取',409,'conflict');if(canvasThread(store,run.projectId))assertCanvasThread(store,run.projectId,typeof b.threadId==='string'?b.threadId:undefined);return response(store.claimHostRun(parts[1],{claimId:string(b.claimId,'claimId'),...(b.clientId!==undefined?{clientId:string(b.clientId,'clientId')}:{})},authorized.token));}
   if(parts[3]==='complete'){const f=await formBody(request),file=f.get('file');let assetVersionIds:string[]|undefined;const rawIds=f.get('assetVersionIds');if(rawIds!==null){try{assetVersionIds=JSON.parse(String(rawIds));}catch{throw new AppError('assetVersionIds 必须是 JSON 数组');}assert(Array.isArray(assetVersionIds),'assetVersionIds 必须是数组');}assert(file===null||file instanceof File,'文件无效');return response(await store.completeHostRun(parts[1],{claimToken:string(f.get('claimToken'),'claimToken'),requestId:string(f.get('requestId'),'requestId'),...(file instanceof File?{bytes:Buffer.from(await file.arrayBuffer()),name:file.name}:{}),...(assetVersionIds!==undefined?{assetVersionIds}:{})}));}
   if(parts[3]==='fail'){const b=await jsonBody(request);return response(store.failHostRun(parts[1],{claimToken:string(b.claimToken,'claimToken'),outcome:b.outcome as 'failed'|'unknown',message:string(b.message,'message')}));}
  }
  if(parts[0]==='runs'&&parts.length===3&&parts[2]==='check-result'&&method==='POST'){await jsonBody(request);return response(checkCanvasResult(store,parts[1]));}
  if(parts[0]==='runs'&&parts.length===2&&method==='GET')return response(store.getRun(parts[1]));
  if(parts[0]==='runs'&&parts[2]==='cancel'&&parts.length===3&&method==='POST'){await jsonBody(request);return response(store.cancel(parts[1]));}
  if(parts[0]==='runs'&&parts[2]==='attach'&&parts.length===3&&method==='POST'){const b=await jsonBody(request);return response(store.attach(parts[1],b.assetVersionIds as string[],string(b.sourceNote,'sourceNote')));}
  if(parts[0]==='runs'&&parts[2]==='recover'&&parts.length===3&&method==='POST'){await jsonBody(request);return response(await store.recoverResult(parts[1]));}
  if(parts[0]==='runs'&&parts[2]==='reconcile'&&parts.length===3&&method==='POST'){const b=await jsonBody(request);return response(store.reconcileRun(parts[1],b.outcome as 'not_accepted'|'charged_no_output',string(b.note,'note')));}
  if(route==='settings'&&method==='GET')return response(store.budget());
  if(route==='settings'&&method==='PATCH'){const b=await jsonBody(request);return response(store.saveSettings(b.settings as BudgetSettings));}
  throw new AppError('接口不存在',404);
 }catch(error){const failure=publicError(error);return response({error:failure.error,errorClass:failure.errorClass},failure.status);}
}
export const GET=handler;export const POST=handler;export const PATCH=handler;export const DELETE=handler;
