import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { mkdtempSync, rmSync } from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import sharp from 'sharp';
import { CanvasStore, hash } from '../src/server/store';
import { createNode } from '../src/domain/graph';
import type { Graph, RunPreview } from '../src/domain/types';
import { AppError } from '../src/server/errors';
import { MockProvider } from '../src/server/providers';
import { authorize, session, sessionCookie } from '../src/server/security';
import JSZip from 'jszip';
let store:CanvasStore,dir:string;
beforeEach(()=>{dir=mkdtempSync(path.join(os.tmpdir(),'canvepp-test-'));store=new CanvasStore(dir,{autoRun:false,mockDelayMs:0});});
afterEach(()=>{store.close();rmSync(dir,{recursive:true,force:true});});
async function fixture(){return store.createProject('测试作品',true);}
function confirm(preview:RunPreview,key='confirm-one'){return store.confirm({previewId:preview.id,snapshotHash:preview.snapshotHash,confirmationId:key,idempotencyKey:key,acceptStale:true});}
describe('immutable local workflow and execution ledger',()=>{
 it('A01 runs only the requested node, snapshots assets, and exports real bytes',async()=>{
  const d=await fixture(),node=d.project.graph.nodes.find(n=>n.kind==='edit')!;const preview=store.preview(d.project.id,node.id,d.project.graphRevision,'mock');expect(store.listRuns(d.project.id)).toHaveLength(0);
  const run=confirm(preview);expect(run.inputSnapshot.sourceAssetHash).toBe(d.assets[0].sha256);const done=await store.processRun(run.id);expect(done.status).toBe('succeeded');expect(done.assetVersionIds).toHaveLength(1);expect(store.listRuns(d.project.id)).toHaveLength(1);expect(store.getProject(d.project.id).graph.nodes.find(n=>n.id===node.id)?.selectedAssetVersionId).toBeUndefined();const asset=store.getAsset(done.assetVersionIds[0]);expect(hash(store.assetBytes(asset.id))).toBe(asset.sha256);expect(asset.runId).toBe(run.id);expect(asset.source).toBe('mock');
 });
 it('A03/A04 input edits do not submit and a moving node leaves the run fingerprint unchanged',async()=>{
  const d=await fixture(),node=d.project.graph.nodes.find(n=>n.kind==='edit')!,text=d.project.graph.nodes.find(n=>n.kind==='text')!;const run=confirm(store.preview(d.project.id,node.id,0,'mock'));const graph=structuredClone(d.project.graph);graph.nodes.find(n=>n.id===node.id)!.position.x+=40;graph.nodes.find(n=>n.id===text.id)!.config.text='新的文字';store.patchGraph(d.project.id,0,graph);const done=await store.processRun(run.id);expect(done.draftChanged).toBe(true);expect(done.inputSnapshot.prompt).not.toBe('新的文字');expect(done.fingerprint).toBe(run.fingerprint);expect(store.listRuns(d.project.id)).toHaveLength(1);expect(store.getProject(d.project.id).graph.nodes.find(n=>n.id===node.id)!.stale).toBe(true);
 });
 it('A05 sibling versions preserve old image hashes and require an explicit adoption',async()=>{
  const d=await fixture(),node=d.project.graph.nodes.find(n=>n.kind==='edit')!;const first=await store.processRun(confirm(store.preview(d.project.id,node.id,0,'mock'),'one').id);const old=store.getAsset(first.assetVersionIds[0]);const graph=structuredClone(d.project.graph);graph.nodes.find(n=>n.id===node.id)!.selectedAssetVersionId=old.id;const p=store.patchGraph(d.project.id,0,graph);const second=await store.processRun(confirm(store.preview(p.id,node.id,p.graphRevision,'mock'),'two').id);expect(second.assetVersionIds[0]).not.toBe(old.id);expect(store.getProject(p.id).graph.nodes.find(n=>n.id===node.id)!.selectedAssetVersionId).toBe(old.id);expect(hash(store.assetBytes(old.id))).toBe(old.sha256);
 });
 it('A06 replaying a confirmation, even with a different key, returns one durable run and job',async()=>{
  const d=await fixture(),node=d.project.graph.nodes.find(n=>n.kind==='generate')!,preview=store.preview(d.project.id,node.id,0,'mock');const a=confirm(preview,'a'),b=confirm(preview,'b');expect(a.id).toBe(b.id);expect(store.listRuns(d.project.id)).toHaveLength(1);expect(store.db.prepare('SELECT COUNT(*) as n FROM jobs').get()).toEqual({n:1});await store.processRun(a.id);await store.processRun(a.id);expect(store.getRun(a.id).events.filter(e=>e.state==='submitting')).toHaveLength(1);
 });
 it('A07 restart preserves an uncertain task and never automatically resubmits',async()=>{
  const d=await fixture(),node=d.project.graph.nodes.find(n=>n.kind==='generate')!;const run=confirm(store.preview(d.project.id,node.id,0,'mock'));store.close();store=new CanvasStore(dir,{autoRun:false,mockDelayMs:0});expect(store.getRun(run.id).status).toBe('unknown');expect((store.db.prepare('SELECT state FROM budget WHERE run_id=?').get(run.id) as {state:string}).state).toBe('reserved');await store.processRun(run.id);expect(store.getRun(run.id).status).toBe('unknown');expect(store.listRuns(d.project.id)).toHaveLength(1);
 });
 it('A08 cancelling a queued task stops submission; a late result is kept',async()=>{
  const d=await fixture(),node=d.project.graph.nodes.find(n=>n.kind==='generate')!;const queued=confirm(store.preview(d.project.id,node.id,0,'mock'),'queued');expect(store.cancel(queued.id).status).toBe('cancelled');await store.processRun(queued.id);expect(store.getRun(queued.id).assetVersionIds).toEqual([]);
  let release!:()=>void;const blocker=new Promise<void>(resolve=>{release=resolve;});store.close();const provider={capabilities:new MockProvider().capabilities,async submit(snapshot:Parameters<MockProvider['submit']>[0],source?:Buffer){await blocker;return new MockProvider().submit(snapshot,source);}};store=new CanvasStore(dir,{autoRun:false,mockDelayMs:0,provider});const run=confirm(store.preview(d.project.id,node.id,0,'mock'),'late');const pending=store.processRun(run.id);expect(store.getRun(run.id).status).toBe('running');store.cancel(run.id);release();const done=await pending;expect(done.status).toBe('succeeded');expect(done.lateResult).toBe(true);expect(done.assetVersionIds).toHaveLength(1);
 });
 it('A09 API is off by default and unknown quota or expired pricing blocks previews',async()=>{
  const d=await fixture(),node=d.project.graph.nodes.find(n=>n.kind==='generate')!;expect(()=>store.preview(d.project.id,node.id,0,'api')).toThrow('API 接入默认关闭');expect(store.listRuns(d.project.id)).toHaveLength(0);
 });
 it('A10 atomic reservations stop the second confirmation from exceeding the cap',async()=>{
  store.close();store=new CanvasStore(dir,{autoRun:false,mockDelayMs:0,estimateCost:()=>.6});
  const original=process.env.OPENAI_API_KEY;process.env.OPENAI_API_KEY='test-key-never-sent';try{const settings={...store.budget().settings,apiEnabled:true,organization:'org-test',project:'proj-test',creditsConfirmed:true,creditsExpireAt:'2099-01-01',availableCredits:20,paidBalanceAcknowledged:true,autoRechargeAcknowledged:true,perRunLimit:1,monthlyLimit:1,pricePerImage:.6,priceValidUntil:'2099-01-01'};store.saveSettings(settings);const d=await fixture(),node=d.project.graph.nodes.find(n=>n.kind==='generate')!;const a=store.preview(d.project.id,node.id,0,'api'),b=store.preview(d.project.id,node.id,0,'api');confirm(a,'a');expect(()=>confirm(b,'b')).toThrow('本月预算');expect(store.listRuns(d.project.id)).toHaveLength(1);expect(store.budget().reserved).toBe(.6);}finally{if(original===undefined)delete process.env.OPENAI_API_KEY;else process.env.OPENAI_API_KEY=original;}
 });
 it('A11 saved graph survives reopening and stale revisions fail without overwriting',async()=>{
  const d=await fixture(),graph=structuredClone(d.project.graph);graph.nodes[0].name='存下的名称';const p=store.patchGraph(d.project.id,0,graph);expect(()=>store.patchGraph(p.id,0,d.project.graph)).toThrow('其他窗口');store.close();store=new CanvasStore(dir,{autoRun:false});expect(store.getProject(p.id).graph.nodes[0].name).toBe('存下的名称');expect(store.listRuns(p.id)).toHaveLength(0);
 });
 it('A12 project packages round-trip assets, snapshots and provenance, excluding settings or keys',async()=>{
  const d=await fixture(),node=d.project.graph.nodes.find(n=>n.kind==='edit')!;const done=await store.processRun(confirm(store.preview(d.project.id,node.id,0,'mock')).id);const zipBytes=await store.exportProject(d.project.id),zip=await JSZip.loadAsync(zipBytes);expect(Object.keys(zip.files).some(n=>/sqlite|settings|key/i.test(n))).toBe(false);const imported=await store.importProject(zipBytes);expect(imported.project.id).not.toBe(d.project.id);expect(imported.assets.map(a=>a.sha256).sort()).toEqual(store.listAssets(d.project.id).map(a=>a.sha256).sort());expect(imported.runs[0].inputSnapshot.prompt).toBe(done.inputSnapshot.prompt);expect(imported.runs[0].assetVersionIds.every(a=>imported.assets.some(x=>x.id===a))).toBe(true);
 });
 it('manual attachments are user-confirmed, only accepted while awaiting import',async()=>{
  const d=await fixture(),node=d.project.graph.nodes.find(n=>n.kind==='edit')!;const run=confirm(store.preview(d.project.id,node.id,0,'manual'));expect(run.status).toBe('awaiting_import');const bytes=await sharp({create:{width:20,height:20,channels:3,background:'red'}}).png().toBuffer();const asset=await store.importAsset(bytes,d.project.id,'manual.png');const attached=store.attach(run.id,[asset.id],'用户确认在外部生成');expect(attached.status).toBe('succeeded');expect(store.getAsset(attached.assetVersionIds[0]).source).toBe('manual');expect(store.getAsset(asset.id).source).toBe('import');expect(()=>store.attach(run.id,[asset.id],'again')).toThrow('等待导入');
 });
 it('preview becomes invalid after inputs change and invalid image imports leave no asset records',async()=>{
  const d=await fixture(),node=d.project.graph.nodes.find(n=>n.kind==='generate')!,preview=store.preview(d.project.id,node.id,0,'mock');const graph=structuredClone(d.project.graph);graph.nodes[0].config.text='changed';store.patchGraph(d.project.id,0,graph);expect(()=>confirm(preview)).toThrow('画布已变更');await expect(store.importAsset(Buffer.from('fake jpeg'),d.project.id,'bad.jpg')).rejects.toThrow('安全解码');expect(store.listAssets(d.project.id)).toHaveLength(1);
 });
 it('package import rejects traversal paths and tampered checksums',async()=>{
  const d=await fixture();const zip=await JSZip.loadAsync(await store.exportProject(d.project.id));zip.file('../escape',Buffer.from('x'));await expect(store.importProject(await zip.generateAsync({type:'nodebuffer'}))).rejects.toThrow('不安全路径');const clean=await JSZip.loadAsync(await store.exportProject(d.project.id));clean.file(`assets/${d.assets[0].id}`,Buffer.from('tamper'));await expect(store.importProject(await clean.generateAsync({type:'nodebuffer'}))).rejects.toThrow('hash');
 });
 it('Next internal localhost URL accepts the real loopback Host and matching external Origin only',()=>{const external=new Request('http://localhost:3210/api/session',{headers:{host:'127.0.0.1:3210',origin:'http://127.0.0.1:3210'}});expect(session(store,external,true).csrf).toBeTruthy();expect(()=>session(store,new Request('http://localhost:3210/api/session',{headers:{host:'evil.example:3210',origin:'http://evil.example:3210'}}),true)).toThrow('主机');expect(()=>session(store,new Request('http://localhost:3210/api/session',{headers:{host:'127.0.0.1:3210',origin:'http://localhost:3210'}}),true)).toThrow('来源');});
 it('keeps canvas sessions on different loopback ports independent while accepting the existing primary cookie',()=>{
  const primary=new Request('http://127.0.0.1:3210/api/session'), secondary=new Request('http://127.0.0.1:3211/api/session');
  const first=session(store,primary,true), second=session(store,secondary,true);
  const cookies=[sessionCookie(first.token,primary),sessionCookie(second.token,secondary)].map(cookie=>cookie.split(';')[0]).join('; ');
  expect(sessionCookie(first.token,primary)).toEqual(sessionCookie(first.token));
  const headers={cookie:cookies};
  expect(authorize(store,new Request(primary.url,{headers})).csrf).toBe(first.csrf);
  expect(authorize(store,new Request(secondary.url,{headers})).csrf).toBe(second.csrf);
 });
 it('local session, CSRF and Origin are enforced for reads and writes',()=>{
  const req=new Request('http://127.0.0.1:3210/api/session');const s=session(store,req,true);const cookie=sessionCookie(s.token).split(';')[0];expect(()=>authorize(store,new Request('http://127.0.0.1:3210/api/projects'))).toThrow('会话');expect(()=>authorize(store,new Request('http://127.0.0.1:3210/api/projects',{method:'POST',headers:{cookie}}))).toThrow('会话确认');expect(()=>authorize(store,new Request('http://127.0.0.1:3210/api/projects',{method:'POST',headers:{cookie,'x-csrf-token':s.csrf,origin:'https://evil.example'}}))).toThrow('来源');expect(authorize(store,new Request('http://127.0.0.1:3210/api/projects',{method:'POST',headers:{cookie,'x-csrf-token':s.csrf,origin:'http://127.0.0.1:3210'}})).csrf).toBe(s.csrf);
 });
});

describe('crash, reconciliation and import guards',()=>{
 it('10 save/reopen cycles retain acknowledged data and reject 10 stale-tab writes',async()=>{
  const d=await fixture();for(let i=0;i<10;i++){const project=store.getProject(d.project.id),graph=structuredClone(project.graph);graph.nodes[0].config.text=`保存版本 ${i}`;const saved=store.patchGraph(project.id,project.graphRevision,graph);expect(()=>store.patchGraph(project.id,project.graphRevision,project.graph)).toThrow('其他窗口');store.close();store=new CanvasStore(dir,{autoRun:false,mockDelayMs:0});expect(store.getProject(project.id).graph.nodes[0].config.text).toBe(`保存版本 ${i}`);expect(store.getProject(project.id).graphRevision).toBe(saved.graphRevision);}expect(store.listRuns(d.project.id)).toHaveLength(0);
 });
 it('storage recovery ingests the staged response without another provider submission',async()=>{
  const d=await fixture(),node=d.project.graph.nodes.find(n=>n.kind==='generate')!;let submissions=0;store.close();store=new CanvasStore(dir,{autoRun:false,mockDelayMs:0,provider:{capabilities:new MockProvider().capabilities,async submit(snapshot,source){submissions++;return new MockProvider().submit(snapshot,source);}}});const run=confirm(store.preview(d.project.id,node.id,0,'mock'));const original=store.importAsset.bind(store);store.importAsset=async()=>{throw new Error('injected disk error');};expect((await store.processRun(run.id)).status).toBe('unknown');expect(store.getRun(run.id).errorClass).toBe('storage_error');store.importAsset=original;const recovered=await store.recoverResult(run.id);expect(recovered.status).toBe('succeeded');expect(submissions).toBe(1);expect(recovered.assetVersionIds).toHaveLength(1);
 });
 it('deleting a running node keeps late output, frozen inputs and history',async()=>{
  const d=await fixture(),node=d.project.graph.nodes.find(n=>n.kind==='generate')!;let release!:()=>void;const blocker=new Promise<void>(resolve=>{release=resolve;});store.close();store=new CanvasStore(dir,{autoRun:false,mockDelayMs:0,provider:{capabilities:new MockProvider().capabilities,async submit(snapshot,source){await blocker;return new MockProvider().submit(snapshot,source);}}});const run=confirm(store.preview(d.project.id,node.id,0,'mock'));const pending=store.processRun(run.id);store.patchGraph(d.project.id,0,{...d.project.graph,nodes:d.project.graph.nodes.filter(n=>n.id!==node.id),edges:d.project.graph.edges.filter(e=>e.sourceNodeId!==node.id&&e.targetNodeId!==node.id)});release();const done=await pending;expect(done.status).toBe('succeeded');expect(done.draftChanged).toBe(true);expect(store.getAsset(done.assetVersionIds[0]).nodeId).toBe(node.id);
 });
 it('unknown tasks require explicit supplier reconciliation before releasing a reservation',async()=>{
  const d=await fixture(),node=d.project.graph.nodes.find(n=>n.kind==='generate')!,run=confirm(store.preview(d.project.id,node.id,0,'mock'));store.close();store=new CanvasStore(dir,{autoRun:false});expect(()=>store.reconcileRun(run.id,'not_accepted','')).toThrow('核对说明');const result=store.reconcileRun(run.id,'not_accepted','已在供应商控制台核对未受理');expect(result.status).toBe('failed');expect(result.usage?.userReconciliation).toBeDefined();expect(store.db.prepare('SELECT state FROM budget WHERE run_id=?').get(run.id)).toEqual({state:'released'});
 });
 it('malformed graphs reject before touching saved state',async()=>{
  const d=await fixture();expect(()=>store.patchGraph(d.project.id,0,{nodes:[null],edges:[]} as unknown as Graph)).toThrow(AppError);expect(store.getProject(d.project.id).graphRevision).toBe(0);
 });
});
