import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { mkdtempSync, rmSync } from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import sharp from 'sharp';
import { CanvasStore, hash } from '../src/server/store';
import { createNode } from '../src/domain/graph';
import type { CanvasNode, Graph, RunPreview } from '../src/domain/types';
let dir:string,store:CanvasStore;
beforeEach(()=>{dir=mkdtempSync(path.join(os.tmpdir(),'canvepp-image-first-'));store=new CanvasStore(dir,{autoRun:false,mockDelayMs:0});});
afterEach(()=>{store.close();rmSync(dir,{recursive:true,force:true});});
const confirm=(preview:RunPreview)=>store.confirm({previewId:preview.id,snapshotHash:preview.snapshotHash,idempotencyKey:crypto.randomUUID(),confirmationId:crypto.randomUUID(),acceptStale:true});
async function imageFixture(withChild=false){
 const detail=await store.createProject('直接编辑图片'),node=createNode('image',detail.project.id);node.config={text:'保留主体，将光线变得柔和',params:{size:'1024x1024',quality:'low',n:1}};
 const child=withChild?createNode('edit',detail.project.id):undefined;if(child)child.config.text='将背景处理成干净的浅灰色';
 const graph:Graph={nodes:child?[node,child]:[node],edges:child?[{id:'image-to-child',sourceNodeId:node.id,sourcePort:'out',targetNodeId:child.id,targetPort:'image'}]:[]};store.patchGraph(detail.project.id,0,graph);
 const bytes=await sharp({create:{width:80,height:60,channels:3,background:'#c7c7c7'}}).png().toBuffer(),original=await store.importAsset(bytes,detail.project.id,'original.png','strip',node.id);const next=structuredClone(store.getProject(detail.project.id).graph);next.nodes.find(n=>n.id===node.id)!.selectedAssetVersionId=original.id;store.patchGraph(detail.project.id,1,next);return {projectId:detail.project.id,node,child,original};
}
function change(projectId:string,update:(nodes:CanvasNode[])=>void){const project=store.getProject(projectId),graph=structuredClone(project.graph);update(graph.nodes);return store.patchGraph(project.id,project.graphRevision,graph);}
function preview(projectId:string,nodeId:string,mode:'mock'|'codex'='mock'){return store.preview(projectId,nodeId,store.getProject(projectId).graphRevision,mode);}

describe('image-first project and immutable runs',()=>{
 it('creates an empty single-card template while retaining legacy and empty project creation',async()=>{
  const created=await store.createProject('新图片',false,'image');expect(created.project.graph.nodes).toHaveLength(1);expect(created.project.graph.nodes[0].kind).toBe('generate');expect(created.project.graph.nodes[0].config.text).toBe('');expect(created.project.graph.edges).toEqual([]);expect(created.assets).toEqual([]);expect(created.runs).toEqual([]);expect(()=>preview(created.project.id,created.project.graph.nodes[0].id)).toThrow();
  const legacy=await store.createProject('旧示例',true);expect(legacy.project.graph.nodes).toHaveLength(6);expect(legacy.assets).toHaveLength(1);expect((await store.createProject('空项目')).project.graph.nodes).toEqual([]);const count=store.listProjects().length;await expect(store.createProject('bad',false,'unknown' as 'image')).rejects.toThrow('项目模板');expect(store.listProjects()).toHaveLength(count);
 });
 it('freezes inline prompt and params without wires, then reports later draft changes',async()=>{
  const created=await store.createProject('独立生成',false,'image'),node=created.project.graph.nodes[0];change(created.project.id,nodes=>{nodes[0].config={text:'一个放在白色桌面上的玻璃杯',params:{size:'1536x1024',quality:'low',n:1}};});
  const frozen=preview(created.project.id,node.id);expect(frozen.snapshot).toMatchObject({nodeKind:'generate',prompt:'一个放在白色桌面上的玻璃杯',params:{size:'1536x1024',quality:'low',n:1}});expect(frozen.snapshot.sourceAssetVersionId).toBeUndefined();const run=confirm(frozen);change(created.project.id,nodes=>{nodes[0].config.text='改成陶瓷杯';nodes[0].config.params!.size='1024x1536';});const done=await store.processRun(run.id);expect(done.status).toBe('succeeded');expect(done.draftChanged).toBe(true);expect(done.inputSnapshot).toEqual(frozen.snapshot);const asset=store.getAsset(done.assetVersionIds[0]);expect([asset.width,asset.height]).toEqual([1536,1024]);expect(store.getProject(created.project.id).graph.nodes[0].selectedAssetVersionId).toBeUndefined();
 });
 it('previews native inline image editing without a request and mocks against the frozen original',async()=>{
  const f=await imageFixture(),native=preview(f.projectId,f.node.id,'codex');expect(native.snapshot).toMatchObject({nodeId:f.node.id,nodeKind:'edit',sourceAssetVersionId:f.original.id,sourceAssetHash:f.original.sha256,provider:'codex',model:'codex-native-imagegen',prompt:f.node.config.text,params:f.node.config.params});expect(store.listRuns(f.projectId)).toEqual([]);expect(store.pendingHostRuns(f.projectId)).toEqual([]);
  const run=confirm(preview(f.projectId,f.node.id));change(f.projectId,nodes=>{nodes.find(n=>n.id===f.node.id)!.config.text='改成夜景';});const done=await store.processRun(run.id);expect(done.status).toBe('succeeded');expect(done.draftChanged).toBe(true);expect(done.inputSnapshot.sourceAssetVersionId).toBe(f.original.id);expect(store.getProject(f.projectId).graph.nodes[0].selectedAssetVersionId).toBe(f.original.id);expect(store.getAsset(done.assetVersionIds[0]).parentVersionIds).toEqual([f.original.id]);expect(hash(store.assetBytes(f.original.id))).toBe(f.original.sha256);expect(store.listAssets(f.projectId)).toHaveLength(2);
 });
 it('adopts an image result as the next editing source and invalidates downstream edits',async()=>{
  const f=await imageFixture(true),child=f.child!;const childRun=await store.processRun(confirm(preview(f.projectId,child.id)).id);change(f.projectId,nodes=>{nodes.find(n=>n.id===child.id)!.selectedAssetVersionId=childRun.assetVersionIds[0];});expect(store.getProject(f.projectId).graph.nodes.find(n=>n.id===child.id)!.stale).toBe(false);
  const done=await store.processRun(confirm(preview(f.projectId,f.node.id)).id);const adopted=change(f.projectId,nodes=>{nodes.find(n=>n.id===f.node.id)!.selectedAssetVersionId=done.assetVersionIds[0];});expect(adopted.graph.nodes.find(n=>n.id===f.node.id)!.stale).toBe(false);expect(adopted.graph.nodes.find(n=>n.id===child.id)!.stale).toBe(true);const next=preview(f.projectId,f.node.id);expect(next.snapshot.sourceAssetVersionId).toBe(done.assetVersionIds[0]);expect(next.snapshot.sourceAssetHash).toBe(store.getAsset(done.assetVersionIds[0]).sha256);expect(hash(store.assetBytes(f.original.id))).toBe(f.original.sha256);
 });
 it('keeps an adopted P1 result stale when the image draft changed to P2 before adoption',async()=>{
  const f=await imageFixture(),done=await store.processRun(confirm(preview(f.projectId,f.node.id)).id);change(f.projectId,nodes=>{nodes[0].config.text='P2：将背景换成夜色';});expect(store.getProject(f.projectId).graph.nodes[0].stale).toBe(true);const adopted=change(f.projectId,nodes=>{nodes[0].selectedAssetVersionId=done.assetVersionIds[0];});expect(adopted.graph.nodes[0].stale).toBe(true);expect(preview(f.projectId,f.node.id).snapshot.prompt).toBe('P2：将背景换成夜色');
 });
 it('does not clear image stale when adopting a version and changing controls in the same save',async()=>{
  const f=await imageFixture(),done=await store.processRun(confirm(preview(f.projectId,f.node.id)).id);const saved=change(f.projectId,nodes=>{nodes[0].config.params!.quality='high';nodes[0].selectedAssetVersionId=done.assetVersionIds[0];});expect(saved.graph.nodes[0].stale).toBe(true);const other=await store.importAsset(await sharp({create:{width:12,height:14,channels:3,background:'white'}}).png().toBuffer(),f.projectId,'new-reference.png');const switched=change(f.projectId,nodes=>{nodes[0].selectedAssetVersionId=other.id;});expect(switched.graph.nodes[0].stale).toBe(false);
 });
});
