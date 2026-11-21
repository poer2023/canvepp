import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { mkdtempSync, rmSync } from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { randomUUID } from 'node:crypto';
import sharp from 'sharp';
import { CanvasStore } from '../src/server/store';
import { confirmGenerationGroup, previewGenerationGroup } from '../src/server/generation-groups';
import { createNode, DEFAULT_PARAMS } from '../src/domain/graph';
import { generationGroup } from '../src/domain/generation-group';
import { findOpenRow } from '../src/lib/image-editor';
import { copySelection, deleteSelection, pasteSelection, restoreGraphSnapshot } from '../src/lib/canvas-editing';
import type { GenerationGroupPreview } from '../src/domain/generation-group';
import type { RunMode } from '../src/domain/types';

let store: CanvasStore, dir: string;
const pixels = (color='#aaa') => sharp({create:{width:64,height:64,channels:3,background:color}}).png().toBuffer();
beforeEach(()=>{dir=mkdtempSync(path.join(os.tmpdir(),'canvepp-groups-'));store=new CanvasStore(dir,{autoRun:false,mockDelayMs:0});});
afterEach(()=>{store.close();rmSync(dir,{recursive:true,force:true});});
async function fixture(count=2){
    const initial=await store.createProject('多图测试');
    const nodes=[];
    for(let i=0;i<count;i++){
        const node=createNode('image',initial.project.id,{x:20,y:20+i*440});
        const asset=await store.importAsset(await pixels(i?'#ddd':'#999'),initial.project.id,`参考 ${i+1}.png`);
        node.selectedAssetVersionId=asset.id;node.config={text:'保留主体，探索不同构图',params:{...DEFAULT_PARAMS,size:'128x128',n:3}};nodes.push(node);
    }
    const project=store.patchGraph(initial.project.id,0,{nodes,edges:[]});
    return {project,nodes,assets:store.listAssets(project.id)};
}
const confirm=(preview:GenerationGroupPreview,key=randomUUID())=>confirmGenerationGroup(store,{previewId:preview.id,snapshotHash:preview.snapshotHash,confirmationKey:key,acceptStale:true});
describe('generation groups',()=>{
    it.each(['image','generate','edit'] as const)('shows a single transparent result from a %s card without replacing the source or duplicating a retry',async(kind)=>{
        const {project,nodes,assets}=await fixture(1);
        const source=createNode(kind,project.id,{x:380,y:20});
        const displayed=await store.importAsset(await pixels('#456'),project.id,'当前显示的图片.png');
        source.selectedAssetVersionId=displayed.id;
        source.config={text:'去掉背景，仅保留人物',params:{...DEFAULT_PARAMS,n:1}};
        const edges=kind==='image'?[]:[{id:randomUUID(),sourceNodeId:nodes[0].id,sourcePort:'out',targetNodeId:source.id,targetPort:kind==='edit'?'image':'reference'}];
        const current=store.patchGraph(project.id,project.graphRevision,{nodes:[...nodes,source],edges});
        const preview=previewGenerationGroup(store,{projectId:project.id,graphRevision:current.graphRevision,mode:'codex',sourceNodeIds:[source.id]});
        expect(preview.nodes).toHaveLength(1);
        expect(preview.references.map(asset=>asset.id)).toEqual([displayed.id]);
        expect(preview.snapshots[0].sourceAssetVersionIds).not.toContain(assets[0].id);
        const group=confirm(preview),run=group.runs[0];
        expect(group.project.graph.nodes.find(node=>node.id===source.id)).toEqual(source);
        expect(group.project.graph.nodes.find(node=>node.id===run.nodeId)?.selectedAssetVersionId).toBeUndefined();
        expect(group.project.graph.edges).toContainEqual(expect.objectContaining({sourceNodeId:source.id,targetNodeId:run.nodeId,targetPort:'reference'}));
        const claim=store.claimHostRun(run.id,{claimId:'single-result'},'owner');
        const bytes=await sharp({create:{width:64,height:64,channels:4,background:{r:20,g:40,b:60,alpha:.5}}}).png().toBuffer();
        const completed=await store.completeHostRun(run.id,{claimToken:claim.claimToken,requestId:'transparent',bytes});
        const after=store.detail(project.id);
        expect(after.project.graph.nodes.find(node=>node.id===run.nodeId)?.selectedAssetVersionId).toBe(completed.assetVersionIds[0]);
        expect(after.project.graph.nodes.filter(node=>current.graph.nodes.some(original=>original.id===node.id))).toEqual(current.graph.nodes);
        expect(after.project.graph.nodes).toHaveLength(current.graph.nodes.length+1);
        expect(await sharp(store.assetBytes(completed.assetVersionIds[0])).metadata()).toMatchObject({hasAlpha:true});
        expect((await sharp(store.assetBytes(completed.assetVersionIds[0])).stats()).isOpaque).toBe(false);
        await store.completeHostRun(run.id,{claimToken:claim.claimToken,requestId:'same-result-again',bytes});
        expect(confirm(preview).runs.map(item=>item.id)).toEqual([run.id]);
        expect(store.detail(project.id)).toEqual(after);
    });
    it('copies a whole reference family and restores deleted copies without losing links, prompts or image versions',async()=>{
        const {project,nodes}=await fixture();const preview=previewGenerationGroup(store,{projectId:project.id,graphRevision:project.graphRevision,mode:'mock',sourceNodeIds:nodes.map(n=>n.id),prompt:'一起参考，保持细节',params:{...DEFAULT_PARAMS,n:2}});
        const group=confirm(preview);for(const run of group.runs)await store.processRun(run.id);
        const graph=store.getProject(project.id).graph,clipboard=copySelection(graph,project.id,graph.nodes.map(n=>n.id))!;
        const pasted=pasteSelection(graph,clipboard,{projectId:project.id});expect(pasted.nodeIds).toHaveLength(4);expect(pasted.graph.edges).toHaveLength(8);
        for(let i=0;i<4;i++){const copy=pasted.graph.nodes.find(n=>n.id===pasted.nodeIds[i])!;expect(copy.config).toEqual(graph.nodes[i].config);expect(copy.selectedAssetVersionId).toBe(graph.nodes[i].selectedAssetVersionId);}
        const deleted=deleteSelection(pasted.graph,pasted.nodeIds).graph;expect(deleted.nodes).toEqual(graph.nodes);expect(deleted.edges).toEqual(graph.edges);
        expect(restoreGraphSnapshot(deleted,pasted.graph,project.id)).toEqual(pasted.graph);expect(restoreGraphSnapshot(pasted.graph,graph,project.id)).toEqual(graph);
        expect(store.listRuns(project.id)).toHaveLength(2);expect(store.listAssets(project.id)).toHaveLength(4);
    });
    it('keeps a batch in one row around obstacles and allows a blank source with provenance lines',async()=>{
        const initial=await store.createProject('空白起稿'),node=createNode('generate',initial.project.id,{x:0,y:0});
        node.config={text:'湖面上的小屋',params:{...DEFAULT_PARAMS,n:4}};
        const blocker=createNode('generate',initial.project.id,{x:650,y:0});
        const positions=findOpenRow([node,blocker],{x:330,y:0},4);
        expect(new Set(positions.map(p=>p.y)).size).toBe(1);
        for(const p of positions)expect(p.x+246<=650||p.x>=896||p.y+420<=0||p.y>=420).toBe(true);
        const project=store.patchGraph(initial.project.id,0,{nodes:[node,blocker],edges:[]});
        const preview=previewGenerationGroup(store,{projectId:project.id,graphRevision:project.graphRevision,mode:'codex',sourceNodeIds:[node.id]});
        expect(preview.references).toEqual([]);expect(preview.edges).toHaveLength(4);expect(preview.edges.every(e=>e.targetPort==='origin')).toBe(true);
        expect(confirm(preview).runs.every(run=>!run.inputSnapshot.sourceAssetVersionId&&run.inputSnapshot.params.n===1)).toBe(true);
    });
    it('creates independent linked outputs atomically and repeats confirmation without duplicating jobs',async()=>{
        const {project,nodes}=await fixture(1),before=store.detail(project.id);
        const preview=previewGenerationGroup(store,{projectId:project.id,graphRevision:project.graphRevision,mode:'mock',sourceNodeIds:[nodes[0].id]});
        expect(store.getProject(project.id)).toEqual(project);
        expect(preview.params.n).toBe(3);expect(preview.nodes).toHaveLength(3);
        expect(new Set(preview.nodes.map(node=>node.position.y)).size).toBe(1);
        expect(preview.nodes[1].position.x-preview.nodes[0].position.x).toBe(290);
        const key=randomUUID(),group=confirm(preview,key),again=confirm(preview,key);
        expect(again.runs.map(run=>run.id)).toEqual(group.runs.map(run=>run.id));expect(store.listRuns(project.id)).toHaveLength(3);
        expect(store.getProject(project.id).graph.edges).toHaveLength(3);
        for(const run of group.runs)await store.processRun(run.id);
        const after=store.detail(project.id);
        expect(after.project.graph.nodes.find(n=>n.id===nodes[0].id)).toEqual(before.project.graph.nodes[0]);
        expect(after.assets).toEqual(expect.arrayContaining(before.assets));
        for(const run of group.runs){const done=store.getRun(run.id);expect(done.status).toBe('succeeded');expect(after.project.graph.nodes.find(n=>n.id===run.nodeId)?.selectedAssetVersionId).toBe(done.assetVersionIds[0]);expect(done.usage?.generationGroup).toMatchObject({total:3,resultCard:true});}
    });
    it('freezes every reference in order, claims each output once, and preserves late or failed output independently',async()=>{
        const {project,nodes}=await fixture();
        const preview=previewGenerationGroup(store,{projectId:project.id,graphRevision:project.graphRevision,mode:'codex',sourceNodeIds:[nodes[1].id,nodes[0].id],prompt:'图 1 的主体，图 2 的环境',params:{...DEFAULT_PARAMS,n:3}});
        const group=confirm(preview),[first,second,third]=group.runs;
        expect(first.inputSnapshot.sourceAssetVersionIds).toEqual([nodes[1].selectedAssetVersionId,nodes[0].selectedAssetVersionId]);
        expect(group.project.graph.edges).toHaveLength(6);
        const a=store.claimHostRun(first.id,{claimId:'first'},'owner');expect(a.canGenerate).toBe(true);expect(store.claimHostRun(first.id,{claimId:'first'},'owner').canGenerate).toBe(false);
        const done=await store.completeHostRun(first.id,{claimToken:a.claimToken,requestId:'first-result',bytes:await pixels()});
        expect(store.getAsset(done.assetVersionIds[0]).parentVersionIds).toEqual(first.inputSnapshot.sourceAssetVersionIds);
        const b=store.claimHostRun(second.id,{claimId:'second'},'owner');store.cancel(second.id);const late=await store.completeHostRun(second.id,{claimToken:b.claimToken,requestId:'late-result',bytes:await pixels('#ccc')});expect(late.lateResult).toBe(true);
        expect(store.getProject(project.id).graph.nodes.find(n=>n.id===second.nodeId)?.selectedAssetVersionId).toBeUndefined();
        const c=store.claimHostRun(third.id,{claimId:'third'},'owner');store.failHostRun(third.id,{claimToken:c.claimToken,outcome:'failed',message:'未调用模型'});
        expect(store.getRun(first.id).status).toBe('succeeded');expect(store.getAsset(late.assetVersionIds[0])).toBeDefined();expect(store.listAssets(project.id)).toHaveLength(4);
    });
    it('does not overwrite an edited output card or recreate a deleted one when results arrive',async()=>{
        const {project,nodes}=await fixture(1);const preview=previewGenerationGroup(store,{projectId:project.id,graphRevision:project.graphRevision,mode:'codex',sourceNodeIds:[nodes[0].id]});
        const {runs}=confirm(preview);const claims=runs.map((run,index)=>store.claimHostRun(run.id,{claimId:`c${index}`},'owner'));
        let p=store.getProject(project.id);p.graph.nodes=p.graph.nodes.filter(n=>n.id!==runs[1].nodeId).map(n=>n.id===runs[0].nodeId?{...n,config:{...n.config,text:'正在编辑的新想法'}}:n);p.graph.edges=p.graph.edges.filter(e=>e.targetNodeId!==runs[1].nodeId);store.patchGraph(p.id,p.graphRevision,p.graph);
        for(let i=0;i<2;i++)await store.completeHostRun(runs[i].id,{claimToken:claims[i].claimToken,requestId:`r${i}`,bytes:await pixels()});
        p=store.getProject(p.id);expect(p.graph.nodes.find(n=>n.id===runs[0].nodeId)).toMatchObject({config:{text:'正在编辑的新想法'}});expect(p.graph.nodes.find(n=>n.id===runs[0].nodeId)?.selectedAssetVersionId).toBeUndefined();expect(p.graph.nodes.some(n=>n.id===runs[1].nodeId)).toBe(false);expect(store.listAssets(p.id)).toHaveLength(3);
    });
    it('rejects a stale preview and rolls back cards and all runs if a child confirmation fails',async()=>{
        const {project,nodes}=await fixture(1),request={projectId:project.id,graphRevision:project.graphRevision,mode:'codex' as const,sourceNodeIds:[nodes[0].id]};
        const preview=previewGenerationGroup(store,request),original=store.confirm.bind(store);let calls=0;
        store.confirm=input=>{if(++calls===2)throw Error('fixture failure');return original(input);};expect(()=>confirm(preview)).toThrow('fixture failure');store.confirm=original;
        expect(store.getProject(project.id)).toEqual(project);expect(store.listRuns(project.id)).toEqual([]);
        store.patchGraph(project.id,project.graphRevision,project.graph,'改名');expect(()=>confirm(preview)).toThrow('画布已变更');expect(store.listRuns(project.id)).toEqual([]);
    });
    it('retains ordered references through backup import, without transferring generation authority',async()=>{
        const {project,nodes}=await fixture();const preview=previewGenerationGroup(store,{projectId:project.id,graphRevision:project.graphRevision,mode:'codex',sourceNodeIds:nodes.map(n=>n.id),prompt:'共同参考',params:{...DEFAULT_PARAMS,n:1}});const group=confirm(preview);
        const restored=await store.importProject(await store.exportProject(project.id));const run=restored.runs[0];
        expect(run.status).toBe('unknown');expect(run.inputSnapshot.sourceAssetVersionIds).toHaveLength(2);
        expect(run.inputSnapshot.sourceAssetVersionIds?.every(id=>restored.assets.some(a=>a.id===id))).toBe(true);
        expect(run.inputSnapshot.sourceAssetVersionIds).not.toEqual(group.runs[0].inputSnapshot.sourceAssetVersionIds);
        expect(generationGroup(run)?.id).not.toBe(group.id);expect(generationGroup(run)?.sourceNodeIds.every(id=>restored.project.graph.nodes.some(n=>n.id===id))).toBe(true);
        expect(()=>store.claimHostRun(run.id,{claimId:'restored'},'owner')).toThrow();
    });
    it('blocks empty reference cards, too many references, and unsupported API without creating outputs',async()=>{
        const {project,nodes}=await fixture();const base={projectId:project.id,graphRevision:project.graphRevision,mode:'codex' as RunMode,sourceNodeIds:nodes.map(n=>n.id),prompt:'共同参考',params:{...DEFAULT_PARAMS,n:2}};
        expect(()=>previewGenerationGroup(store,{...base,mode:'api'})).toThrow('独立 API');
        const p=store.getProject(project.id);delete p.graph.nodes[1].selectedAssetVersionId;const next=store.patchGraph(p.id,p.graphRevision,p.graph);
        expect(()=>previewGenerationGroup(store,{...base,graphRevision:next.graphRevision})).toThrow('空白');
        expect(()=>previewGenerationGroup(store,{...base,graphRevision:next.graphRevision,sourceNodeIds:Array.from({length:9},(_,i)=>String(i))})).toThrow('1–8');
        expect(store.listRuns(p.id)).toHaveLength(0);
    });
    it('a reference change after confirmation does not replace frozen inputs and keeps the result in history',async()=>{
        const {project,nodes}=await fixture();const preview=previewGenerationGroup(store,{projectId:project.id,graphRevision:project.graphRevision,mode:'codex',sourceNodeIds:nodes.map(n=>n.id),prompt:'共同参考',params:{...DEFAULT_PARAMS,n:1}});const {runs:[run]}=confirm(preview);
        const replacement=await store.importAsset(await pixels('#222'),project.id,'new.png');const p=store.getProject(project.id);p.graph.nodes[0].selectedAssetVersionId=replacement.id;store.patchGraph(p.id,p.graphRevision,p.graph);
        const claim=store.claimHostRun(run.id,{claimId:'frozen'},'owner');expect(claim.run.inputSnapshot.sourceAssetVersionIds).toEqual(preview.references.map(a=>a.id));
        const done=await store.completeHostRun(run.id,{claimToken:claim.claimToken,requestId:'frozen-result',bytes:await pixels()});expect(done.draftChanged).toBe(true);expect(store.getProject(p.id).graph.nodes.find(n=>n.id===run.nodeId)?.selectedAssetVersionId).toBeUndefined();
    });
});
