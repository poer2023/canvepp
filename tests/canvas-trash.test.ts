import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { mkdtempSync, rmSync } from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import sharp from 'sharp';
import JSZip from 'jszip';
import { CanvasStore } from '../src/server/store';
import { createNode, DEFAULT_PARAMS } from '../src/domain/graph';
import { listTrash, restoreTrashedCards } from '../src/server/trash';
import { deleteSelection } from '../src/lib/canvas-editing';
import type { Project } from '../src/domain/types';
let store:CanvasStore,dir:string;
beforeEach(()=>{dir=mkdtempSync(path.join(os.tmpdir(),'canvepp-trash-'));store=new CanvasStore(dir,{autoRun:false});});
afterEach(()=>{store.close();rmSync(dir,{recursive:true,force:true});});
const image=()=>sharp({create:{width:20,height:24,channels:3,background:'#ccc'}}).png().toBuffer();
async function fixture(){
 const {project}=await store.createProject('回收站测试',false);
 const a=createNode('image',project.id,{x:10,y:20}),b=createNode('generate',project.id,{x:400,y:20});
 a.config={text:'原始提示词',params:{...DEFAULT_PARAMS,size:'1920x1080'},note:'保留备注'};b.config.text='第二张';
 let p=store.patchGraph(project.id,0,{nodes:[a,b],edges:[]});
 const asset=await store.importAsset(await image(),p.id,'原始.png','preserve',a.id);a.selectedAssetVersionId=asset.id;b.selectedAssetVersionId=asset.id;
 p=store.patchGraph(p.id,p.graphRevision,{nodes:[a,b],edges:[{id:'reference-link',sourceNodeId:a.id,sourcePort:'out',targetNodeId:b.id,targetPort:'reference'}]});
 return {p,a:p.graph.nodes[0],b:p.graph.nodes[1],asset};
}
function remove(p:Project,ids:string[]){return store.patchGraph(p.id,p.graphRevision,deleteSelection(p.graph,ids).graph);}
describe('persistent image recycle bin',()=>{
 it('retains full cards, immutable versions and links across reopening, then restores without generation',async()=>{
  const {p,a,b,asset}=await fixture(),bytes=store.assetBytes(asset.id);remove(p,[a.id,b.id]);
  let entries=listTrash(store,p.id);expect(entries).toHaveLength(2);expect(entries.find(e=>e.node.id===a.id)?.node).toEqual(a);
  store.close();store=new CanvasStore(dir,{autoRun:false});entries=listTrash(store,p.id);expect(entries).toHaveLength(2);
  const restored=restoreTrashedCards(store,p.id,entries.map(e=>e.id),store.getProject(p.id).graphRevision);
  expect(restored.project.graph.nodes.find(n=>n.id===a.id)?.config).toEqual(a.config);expect(restored.project.graph.nodes.map(n=>n.selectedAssetVersionId)).toEqual([asset.id,asset.id]);expect(restored.project.graph.edges).toEqual(p.graph.edges);
  expect(listTrash(store,p.id)).toEqual([]);expect(store.assetBytes(asset.id)).toEqual(bytes);expect(store.listRuns(p.id)).toEqual([]);
  expect(restoreTrashedCards(store,p.id,entries.map(e=>e.id),restored.project.graphRevision).restoredIds).toEqual([]);
 });
 it('undo marks entries restored and redo keeps one active deletion per card',async()=>{
  const {p,a}=await fixture();let current=remove(p,[a.id]);expect(listTrash(store,p.id)).toHaveLength(1);
  current=store.patchGraph(p.id,current.graphRevision,p.graph);expect(listTrash(store,p.id)).toEqual([]);
  current=remove(current,[a.id]);expect(listTrash(store,p.id)).toHaveLength(1);
  const entry=listTrash(store,p.id)[0];restoreTrashedCards(store,p.id,[entry.id],current.graphRevision);expect(listTrash(store,p.id)).toEqual([]);
 });
 it('restores saved links when both endpoints return in separate operations',async()=>{
  const {p,a,b}=await fixture();const deleted=remove(p,[a.id,b.id]),entries=listTrash(store,p.id);
  const first=restoreTrashedCards(store,p.id,[entries.find(e=>e.node.id===b.id)!.id],deleted.graphRevision);expect(first.project.graph.edges).toHaveLength(0);
  const second=restoreTrashedCards(store,p.id,[entries.find(e=>e.node.id===a.id)!.id],first.project.graphRevision);expect(second.project.graph.edges).toEqual(p.graph.edges);
 });
 it('does not clobber a changed input link or silently discard its retained original',async()=>{
  const {p,a,b}=await fixture();let deleted=remove(p,[a.id]);const entry=listTrash(store,p.id)[0];const c=createNode('image',p.id,{x:800,y:20});
  const replacement={id:'replacement',sourceNodeId:c.id,sourcePort:'out',targetNodeId:b.id,targetPort:'reference'};
  deleted=store.patchGraph(p.id,deleted.graphRevision,{...deleted.graph,nodes:[...deleted.graph.nodes,c],edges:[replacement]});
  const restored=restoreTrashedCards(store,p.id,[entry.id],deleted.graphRevision);
  // Reference ports allow multiple sources; both remain, so nothing is overwritten.
  expect(restored.project.graph.edges).toEqual(expect.arrayContaining([replacement,...p.graph.edges]));
 });
 it('rejects stale revisions and foreign entries without modifying either canvas',async()=>{
  const {p,a}=await fixture(),removed=remove(p,[a.id]),entry=listTrash(store,p.id)[0];const other=(await store.createProject('其他',false)).project;
  expect(()=>restoreTrashedCards(store,p.id,[entry.id],p.graphRevision)).toThrow('画布已更新');
  expect(()=>restoreTrashedCards(store,other.id,[entry.id],other.graphRevision)).toThrow('回收站图片不存在');
  expect(store.getProject(p.id)).toEqual(removed);expect(store.getProject(other.id)).toEqual(other);
 });
 it('keeps late results with a deleted card and never mistakes unused live versions for trash',async()=>{
  const {p,a,asset}=await fixture();const preview=store.preview(p.id,a.id,p.graphRevision,'manual'),run=store.confirm({previewId:preview.id,snapshotHash:preview.snapshotHash,confirmationId:'manual',idempotencyKey:'manual'});
  const result=await store.importAsset(await image(),p.id,'后续.png','strip',a.id);
  expect(listTrash(store,p.id)).toEqual([]);remove(p,[a.id]);
  const attached=store.attach(run.id,[result.id],'用户确认的隔离测试结果');
  const entry=listTrash(store,p.id)[0];expect(entry.assetVersionIds).toEqual(expect.arrayContaining([asset.id,result.id,...attached.assetVersionIds]));expect(entry.displayAssetVersionId).toBe(asset.id);
  const restored=restoreTrashedCards(store,p.id,[entry.id],store.getProject(p.id).graphRevision);expect(restored.project.graph.nodes.find(n=>n.id===a.id)?.selectedAssetVersionId).toBe(asset.id);
 });
 it('recovers legacy removed owners but excludes assets still used by another card',async()=>{
  const {p,a,b}=await fixture();store.db.prepare('UPDATE projects SET body=? WHERE id=?').run(JSON.stringify({...p,graph:{nodes:[b],edges:[]}}),p.id);expect(listTrash(store,p.id)).toEqual([]);
  store.db.prepare('UPDATE projects SET body=? WHERE id=?').run(JSON.stringify({...p,graph:{nodes:[],edges:[]}}),p.id);const legacy=listTrash(store,p.id)[0];expect(legacy.legacy).toBe(true);expect(legacy.node.id).toBe(a.id);
  restoreTrashedCards(store,p.id,[legacy.id],p.graphRevision);expect(listTrash(store,p.id)).toEqual([]);
 });
 it('includes deleted content and stable link remapping in backups',async()=>{
  const {p,a,b,asset}=await fixture();remove(p,[a.id,b.id]);const backup=await store.exportProject(p.id),imported=await store.importProject(backup);
  const entries=listTrash(store,imported.project.id);expect(entries).toHaveLength(2);expect(entries.every(e=>e.node.projectId===imported.project.id)).toBe(true);expect(entries.every(e=>e.assetVersionIds[0]!==asset.id)).toBe(true);
  const restored=restoreTrashedCards(store,imported.project.id,entries.map(e=>e.id),imported.project.graphRevision);
  expect(restored.project.graph.nodes.map(n=>n.config.text).sort()).toEqual(['原始提示词','第二张']);expect(restored.project.graph.edges).toHaveLength(1);expect(restored.project.graph.edges[0].id).not.toBe(p.graph.edges[0].id);
  expect(restored.project.graph.nodes.some(n=>n.id===a.id||n.id===b.id)).toBe(false);expect(store.assetBytes(imported.assets[0].id)).toEqual(store.assetBytes(asset.id));expect(listTrash(store,p.id)).toHaveLength(2);
 });
 it('imports older backups without a recycle bin and rejects malformed trash atomically',async()=>{
  const {p,a}=await fixture();remove(p,[a.id]);const zip=await JSZip.loadAsync(await store.exportProject(p.id));zip.remove('trash.json');const old=await store.importProject(await zip.generateAsync({type:'nodebuffer'}));expect(old.assets).toHaveLength(1);
  const before=store.listProjects().length;zip.file('trash.json',JSON.stringify([{projectId:p.id,node:{bad:true}}]));await expect(store.importProject(await zip.generateAsync({type:'nodebuffer'}))).rejects.toThrow('回收站图片无效');expect(store.listProjects()).toHaveLength(before);
 });
});
