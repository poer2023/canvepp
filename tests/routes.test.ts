import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { existsSync, mkdtempSync, rmSync } from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import sharp from 'sharp';
import { GET, PATCH, POST } from '../src/app/api/[[...path]]/route';
import { getStore, type CanvasStore } from '../src/server/store';
import { registerHostContext } from '../src/server/host-dispatch';
import { openThreadCanvas } from '../src/server/thread-canvas';
import { randomUUID } from 'node:crypto';
import { POST as openHostCanvas } from '../src/app/api/host/canvas/route';
let dir:string,originalDir:string|undefined,cookie:string,csrf:string,hostToken:string|undefined;
const context=(parts:string[])=>({params:Promise.resolve({path:parts})});
const base='http://127.0.0.1:3210/api/';
function req(parts:string[],method='GET',body?:unknown,extra:Record<string,string>={}){return new Request(base+parts.join('/'),{method,headers:{cookie,'x-csrf-token':csrf,...(hostToken?{'x-canvepp-host':hostToken}:{}),...(body!==undefined?{'content-type':'application/json'}:{}),...extra},body:body!==undefined?JSON.stringify(body):undefined});}
beforeEach(async()=>{hostToken=undefined;originalDir=process.env.CANVEPP_DATA_DIR;dir=mkdtempSync(path.join(os.tmpdir(),'canvepp-route-'));process.env.CANVEPP_DATA_DIR=dir;const result=await GET(new Request(base+'session'),context(['session']));cookie=result.headers.get('set-cookie')!.split(';')[0];csrf=(await result.json()).csrfToken;});
afterEach(()=>{const global=globalThis as unknown as {canveppStore?:CanvasStore};global.canveppStore?.close();delete global.canveppStore;if(originalDir===undefined)delete process.env.CANVEPP_DATA_DIR;else process.env.CANVEPP_DATA_DIR=originalDir;rmSync(dir,{recursive:true,force:true});});
describe('local HTTP boundary',()=>{
 it('scopes native multi-output confirmation to its chat and does not queue host messages implicitly',async()=>{
  const store=getStore(),a=registerHostContext(store,randomUUID()),b=registerHostContext(store,randomUUID());
  const created=await store.createProject('批量归属',true);openThreadCanvas(store,a.threadId,created.project.id);
  const p=store.getProject(created.project.id),node=p.graph.nodes.find(n=>n.kind==='generate')!;
  p.graph.edges=p.graph.edges.filter(e=>e.targetNodeId!==node.id);node.config={text:'测试原生批量，不调用生图',params:{size:'1024x1024',quality:'high',n:2}};
  const saved=store.patchGraph(p.id,p.graphRevision,p.graph),parts=['generation-groups','preview'];
  const input={projectId:p.id,graphRevision:saved.graphRevision,mode:'codex',sourceNodeIds:[node.id],requestId:randomUUID()};
  hostToken=b.token;expect((await POST(req(parts,'POST',input),context(parts))).status).toBe(403);
  hostToken=a.token;const previewResponse=await POST(req(parts,'POST',input),context(parts));expect(previewResponse.status).toBe(200);const preview=await previewResponse.json();
  const confirmation={previewId:preview.id,snapshotHash:preview.snapshotHash,confirmationKey:randomUUID(),acceptStale:true,requestId:randomUUID()};
  hostToken=b.token;expect((await POST(req(['generation-groups'],'POST',confirmation),context(['generation-groups']))).status).toBe(403);expect(store.listRuns(p.id)).toHaveLength(0);
  hostToken=a.token;const result=await POST(req(['generation-groups'],'POST',confirmation),context(['generation-groups']));expect(result.status).toBe(201);const group=await result.json();expect(group.runs).toHaveLength(2);expect(group.runs.every((r:{status:string})=>r.status==='awaiting_host')).toBe(true);
  expect(store.db.prepare('SELECT COUNT(*) AS n FROM host_dispatches').get()).toEqual({n:0});
 });
 it('protects the recycle bin and restores full cards with an expected revision',async()=>{
  const store=getStore(),{project}=await store.createProject('删除恢复',false,'image');
  const removed=store.patchGraph(project.id,project.graphRevision,{nodes:[],edges:[]});
  const parts=['projects',project.id,'trash'],restoreParts=[...parts,'restore'];
  expect((await GET(new Request(base+parts.join('/')),context(parts))).status).toBe(401);
  const listed=await GET(req(parts),context(parts));expect(listed.status).toBe(200);const {items}=await listed.json();expect(items).toHaveLength(1);
  const body={entryIds:[items[0].id],expectedRevision:removed.graphRevision,requestId:randomUUID()};
  expect((await POST(req(restoreParts,'POST',body,{'x-csrf-token':'wrong'}),context(restoreParts))).status).toBe(403);
  expect((await POST(req(restoreParts,'POST',{...body,expectedRevision:0,requestId:randomUUID()}),context(restoreParts))).status).toBe(409);
  const restored=await POST(req(restoreParts,'POST',{...body,requestId:randomUUID()}),context(restoreParts));expect(restored.status).toBe(200);expect((await restored.json()).project.graph.nodes[0].id).toBe(project.graph.nodes[0].id);
  expect(store.listRuns(project.id)).toEqual([]);expect((await (await GET(req(parts),context(parts))).json()).items).toEqual([]);
 });
 it('requires local authentication for cross-canvas image search',async()=>{
  expect((await GET(new Request(base+'image-library'),context(['image-library']))).status).toBe(401);
  const result=await GET(req(['image-library']),context(['image-library']));
  expect(result.status).toBe(200);expect(await result.json()).toEqual({items:[],total:0});
 });
 it('opens one canvas per authenticated chat and refuses cross-chat preview, confirmation and claim',async()=>{
  const store=getStore(),a=registerHostContext(store,randomUUID()),b=registerHostContext(store,randomUUID());
  const endpoint=base+'host/canvas';
  expect((await openHostCanvas(new Request(endpoint,{method:'POST',headers:{'content-type':'application/json'},body:'{}'}))).status).toBe(401);
  hostToken=a.token;
  const opened=await openHostCanvas(req(['host','canvas'],'POST',{requestId:randomUUID()}));expect(opened.status).toBe(200);const first=await opened.json();
  const again=await (await openHostCanvas(req(['host','canvas'],'POST',{requestId:randomUUID()}))).json();expect(again.detail.project.id).toBe(first.detail.project.id);expect(store.listProjects()).toHaveLength(1);
  const project=first.detail.project,node=project.graph.nodes[0];node.config.text='隔离测试，不生成';
  store.patchGraph(project.id,0,project.graph);
  const input={projectId:project.id,nodeId:node.id,graphRevision:1,mode:'codex',requestId:'preview'};
  hostToken=b.token;expect((await POST(req(['runs','preview'],'POST',input),context(['runs','preview']))).status).toBe(403);
  hostToken=a.token;const preview=await (await POST(req(['runs','preview'],'POST',input),context(['runs','preview']))).json();
  const confirm={previewId:preview.id,snapshotHash:preview.snapshotHash,confirmationId:'owner-only',idempotencyKey:'owner-only',requestId:'confirm'};
  hostToken=b.token;expect((await POST(req(['runs'],'POST',confirm),context(['runs']))).status).toBe(403);expect(store.listRuns(project.id)).toEqual([]);
  hostToken=a.token;const response=await POST(req(['runs'],'POST',confirm),context(['runs']));expect(response.status).toBe(201);const run=await response.json();
  const parts=['runs',run.id,'host','claim'];
  expect((await POST(req(parts,'POST',{threadId:b.threadId,claimId:'wrong',requestId:'wrong'}),context(parts))).status).toBe(403);
  expect(store.getRun(run.id).status).toBe('awaiting_host');
  expect((await POST(req(parts,'POST',{threadId:a.threadId,claimId:'right',requestId:'right'}),context(parts))).status).toBe(200);
 });
 it('creates the image-first template through HTTP without adding legacy sample nodes',async()=>{
  const result=await POST(req(['projects'],'POST',{name:'单图片画布',template:'image',requestId:'image-template'}),context(['projects']));expect(result.status).toBe(201);const detail=await result.json();expect(detail.project.graph.nodes).toHaveLength(1);expect(detail.project.graph.nodes[0].kind).toBe('generate');expect(detail.project.graph.edges).toEqual([]);expect(detail.assets).toEqual([]);expect(detail.runs).toEqual([]);const invalid=await POST(req(['projects'],'POST',{name:'invalid',template:'unknown',requestId:'invalid-template'}),context(['projects']));expect(invalid.status).toBe(422);
 });

 it('probes service identity without creating data or requiring a session, while enforcing loopback origin',async()=>{
  const global=globalThis as unknown as {canveppStore?:CanvasStore};global.canveppStore?.close();delete global.canveppStore;const unused=path.join(dir,'health-must-not-open');process.env.CANVEPP_DATA_DIR=unused;
  const result=await GET(new Request(base+'health'),context(['health']));expect(result.status).toBe(200);const body=await result.json();expect(body).toMatchObject({service:'canvepp',protocolVersion:1,pid:process.pid});expect(body.rootId).toMatch(/^[a-f0-9]{64}$/);expect(body.instanceId).toBeTruthy();expect(body).not.toHaveProperty('rootHash');expect(JSON.stringify(body)).not.toContain(process.cwd());expect(existsSync(unused)).toBe(false);expect(global.canveppStore).toBeUndefined();
  const outside=await GET(new Request(base+'health',{headers:{origin:'https://evil.example'}}),context(['health']));expect(outside.status).toBe(403);const evil=await GET(new Request(base+'health',{headers:{host:'evil.example'}}),context(['health']));expect(evil.status).toBe(403);expect(existsSync(unused)).toBe(false);
 });
 it('protects native host routes with session/CSRF and writes authenticated actual result bytes',async()=>{
  const detail=await (await POST(req(['projects'],'POST',{name:'native HTTP',starter:true,requestId:'project'}),context(['projects']))).json(),node=detail.project.graph.nodes.find((n:{kind:string})=>n.kind==='edit');
  const owner=registerHostContext(getStore(),randomUUID());openThreadCanvas(getStore(),owner.threadId,detail.project.id);hostToken=owner.token;
  const preview=await (await POST(req(['runs','preview'],'POST',{projectId:detail.project.id,nodeId:node.id,graphRevision:0,mode:'codex',requestId:'preview'}),context(['runs','preview']))).json();
  const confirmed=await POST(req(['runs'],'POST',{previewId:preview.id,snapshotHash:preview.snapshotHash,confirmationId:'http-native',idempotencyKey:'http-native',requestId:'confirm'}),context(['runs']));expect(confirmed.status).toBe(201);const run=await confirmed.json();expect(run.status).toBe('awaiting_host');
  expect((await GET(new Request(base+'host/pending'),context(['host','pending']))).status).toBe(401);const pending=await (await GET(req(['host','pending']),context(['host','pending']))).json();expect(pending.runs.map((r:{id:string})=>r.id)).toEqual([run.id]);
  const parts=['runs',run.id,'host','claim'];expect((await POST(req(parts,'POST',{threadId:owner.threadId,claimId:'http-claim',requestId:'claim'},{'x-csrf-token':'bad'}),context(parts))).status).toBe(403);const host=await (await POST(req(parts,'POST',{threadId:owner.threadId,claimId:'http-claim',clientId:'http-client',requestId:'claim'}),context(parts))).json();expect(host.canGenerate).toBe(true);expect((await (await POST(req(parts,'POST',{threadId:owner.threadId,claimId:'http-claim',clientId:'http-client',requestId:'reread'}),context(parts))).json()).canGenerate).toBe(false);
  const form=new FormData();form.set('requestId','result-http');form.set('claimToken',host.claimToken);const png=await sharp({create:{width:12,height:13,channels:3,background:'green'}}).png().toBuffer();form.set('file',new Blob([new Uint8Array(png)],{type:'image/png'}),'codex.png');const completeParts=['runs',run.id,'host','complete'];const result=await POST(new Request(base+completeParts.join('/'),{method:'POST',headers:{cookie,'x-csrf-token':csrf},body:form}),context(completeParts));expect(result.status).toBe(200);const done=await result.json();expect(done.status).toBe('succeeded');expect(JSON.stringify(done)).not.toContain(host.claimToken);const detailAfter=await (await GET(req(['projects',detail.project.id]),context(['projects',detail.project.id]))).json();expect(detailAfter.assets.find((a:{id:string})=>a.id===done.assetVersionIds[0]).source).toBe('codex');
 });

 it('refuses cross-site sessions and unauthenticated asset or project reads',async()=>{
  const outside=await GET(new Request(base+'session',{headers:{origin:'https://untrusted.example'}}),context(['session']));expect(outside.status).toBe(403);const asset=await GET(new Request(base+'assets/unknown'),context(['assets','unknown']));expect(asset.status).toBe(401);const projects=await GET(new Request(base+'projects'),context(['projects']));expect(projects.status).toBe(401);
 });
 it('persists an imported image through the authenticated project/asset HTTP contracts',async()=>{
  const created=await POST(req(['projects'],'POST',{name:'HTTP项目',requestId:'create-http'}),context(['projects']));expect(created.status).toBe(201);const detail=await created.json();const png=await sharp({create:{width:8,height:9,channels:3,background:'blue'}}).png().toBuffer();const form=new FormData();form.set('file',new Blob([new Uint8Array(png)],{type:'image/png'}),'upload.png');form.set('projectId',detail.project.id);form.set('requestId','upload-http');const uploaded=await POST(new Request(base+'assets/import',{method:'POST',headers:{cookie,'x-csrf-token':csrf},body:form}),context(['assets','import']));expect(uploaded.status).toBe(201);const asset=await uploaded.json();const image=await GET(req(['assets',asset.id]),context(['assets',asset.id]));expect(image.status).toBe(200);expect(image.headers.get('content-type')).toBe('image/png');expect((await sharp(Buffer.from(await image.arrayBuffer())).metadata()).width).toBe(8);const thumb=await GET(req(['assets',asset.id+'?thumbnail=1']),context(['assets',asset.id]));expect(thumb.status).toBe(200);expect(thumb.headers.get('content-type')).toBe('image/webp');const thumbMeta=await sharp(Buffer.from(await thumb.arrayBuffer())).metadata();expect(thumbMeta.width).toBe(8);expect(thumbMeta.height).toBe(9);
 });
 it('invalid multipart and malformed graphs return actionable validation errors',async()=>{
  const invalid=await POST(new Request(base+'assets/import',{method:'POST',headers:{cookie,'x-csrf-token':csrf,'content-type':'multipart/form-data'},body:'not-a-multipart'}),context(['assets','import']));expect(invalid.status).toBe(422);expect((await invalid.json()).errorClass).toBe('validation');const p=await (await POST(req(['projects'],'POST',{name:'bad graph',requestId:'create'}),context(['projects']))).json();const bad=await PATCH(req(['projects',p.project.id,'graph'],'PATCH',{expectedRevision:0,graph:{nodes:[null],edges:[]},requestId:'invalid-graph'}),context(['projects',p.project.id,'graph']));expect(bad.status).toBe(422);
 });
 it('writes require CSRF and bounded request bodies',async()=>{
  const noCSRF=await POST(req(['projects'],'POST',{name:'blocked',requestId:'blocked'},{'x-csrf-token':'wrong'}),context(['projects']));expect(noCSRF.status).toBe(403);const oversized=await POST(req(['projects'],'POST',{name:'blocked',requestId:'blocked'},{'content-length':String(3*1024*1024)}),context(['projects']));expect(oversized.status).toBe(413);const list=await GET(req(['projects']),context(['projects']));expect((await list.json()).projects).toEqual([]);
 });
});
