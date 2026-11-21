import { randomUUID } from 'node:crypto';
import type { CanvasEdge, Graph, Project, TrashedCard } from '../domain/types';
import { createNode, DEFAULT_PARAMS, getNodeOutputAsset, validateConnection, validateGraph } from '../domain/graph';
import { findOpenPosition } from '../lib/image-editor';
import type { CanvasStore } from './store';
import { assert } from './errors';

function schema(store:CanvasStore) {
    store.db.exec('CREATE TABLE IF NOT EXISTS canvas_trash(id TEXT PRIMARY KEY,project_id TEXT NOT NULL,node_id TEXT NOT NULL,body TEXT NOT NULL,restored_at TEXT); CREATE INDEX IF NOT EXISTS canvas_trash_project ON canvas_trash(project_id,restored_at)');
}
export function retainDeletedCards(store:CanvasStore, previous:Project, next:Graph) {
    const previousIds=new Set(previous.graph.nodes.map(node=>node.id)),nextIds=new Set(next.nodes.map(node=>node.id));
    const removed=previous.graph.nodes.filter(node=>!nextIds.has(node.id)),added=next.nodes.filter(node=>!previousIds.has(node.id));
    if(!removed.length&&!added.length)return;
    schema(store);
    const live=new Set(next.nodes.map(node=>node.id)),assets=store.listAssets(previous.id),runs=store.listRuns(previous.id);
    for(const node of removed) {
        if(live.has(node.id))continue;
        const displayAssetVersionId=getNodeOutputAsset(previous.graph,node.id);
        const assetVersionIds=[...new Set([...assets.filter(asset=>asset.nodeId===node.id).map(asset=>asset.id),...runs.filter(run=>run.nodeId===node.id).flatMap(run=>run.assetVersionIds),...(displayAssetVersionId?[displayAssetVersionId]:[])])];
        const entry:TrashedCard={id:randomUUID(),projectId:previous.id,node,edges:previous.graph.edges.filter(edge=>edge.sourceNodeId===node.id||edge.targetNodeId===node.id),assetVersionIds,displayAssetVersionId,deletedAt:new Date().toISOString()};
        store.db.prepare('INSERT INTO canvas_trash VALUES (?,?,?,?,NULL)').run(entry.id,previous.id,node.id,JSON.stringify(entry));
    }
    // Undo/redo and restoration through another window use the same node IDs.
    for(const node of added)store.db.prepare('UPDATE canvas_trash SET restored_at=? WHERE project_id=? AND node_id=? AND restored_at IS NULL').run(new Date().toISOString(),previous.id,node.id);
}
export function listTrash(store:CanvasStore, projectId:string):TrashedCard[] {
    schema(store);
    const project=store.getProject(projectId),live=new Set(project.graph.nodes.map(n=>n.id));
    const entries=(store.db.prepare('SELECT body FROM canvas_trash WHERE project_id=? AND restored_at IS NULL ORDER BY rowid DESC').all(projectId) as {body:string}[]).map(row=>JSON.parse(row.body) as TrashedCard).filter(entry=>!live.has(entry.node.id));
    const known=new Set((store.db.prepare('SELECT node_id FROM canvas_trash WHERE project_id=?').all(projectId) as {node_id:string}[]).map(row=>row.node_id));
    const used=new Set(project.graph.nodes.map(node=>getNodeOutputAsset(project.graph,node.id)).filter(Boolean));
    const runs=store.listRuns(projectId),assets=store.listAssets(projectId),groups=new Map<string,typeof assets>();
    for(const entry of entries){const returned=runs.filter(run=>run.nodeId===entry.node.id).flatMap(run=>run.assetVersionIds);entry.assetVersionIds=[...new Set([...entry.assetVersionIds,...returned])];entry.displayAssetVersionId??=returned[0];}
    for(const asset of assets){const owner=asset.nodeId||runs.find(run=>run.id===asset.runId)?.nodeId;if(owner&&!live.has(owner)&&!known.has(owner))groups.set(owner,[...(groups.get(owner)||[]),asset]);}
    // Only a known removed card qualifies. Unadopted versions of live cards stay in history.
    for(const [owner,versions] of groups){
        if(versions.some(asset=>used.has(asset.id)))continue;
        const latest=[...versions].sort((a,b)=>b.createdAt.localeCompare(a.createdAt))[0],run=runs.find(run=>run.id===latest.runId);
        const node={...createNode('image',projectId,{x:100,y:100}),id:owner,name:latest.name,selectedAssetVersionId:latest.id,config:{text:run?.inputSnapshot.prompt||'',params:{...(run?.inputSnapshot.params||DEFAULT_PARAMS)}}};
        entries.push({id:`legacy:${owner}`,projectId,node,edges:[],assetVersionIds:versions.map(asset=>asset.id),displayAssetVersionId:latest.id,deletedAt:latest.createdAt,legacy:true});
    }
    return entries;
}
export function restoreTrashedCards(store:CanvasStore,projectId:string,entryIds:string[],expectedRevision:number) {
    schema(store);
    assert(Array.isArray(entryIds)&&entryIds.length>0&&entryIds.length<=500&&entryIds.every(id=>typeof id==='string'),'请选择要恢复的图片');
    return store.db.transaction(()=>{
        const project=store.getProject(projectId);assert(project.graphRevision===expectedRevision,'画布已更新，请同步后恢复',409,'conflict');
        const all=listTrash(store,projectId),entries=entryIds.map(id=>all.find(entry=>entry.id===id)).filter(Boolean) as TrashedCard[];
        // Repeating restoration is harmless. Unknown IDs may belong to another canvas.
        for(const id of entryIds)if(!entries.some(e=>e.id===id))assert(store.db.prepare('SELECT 1 FROM canvas_trash WHERE id=? AND project_id=? AND restored_at IS NOT NULL').get(id,projectId),'回收站图片不存在',404);
        const nodes=[...project.graph.nodes],edges=[...project.graph.edges],restoredIds:string[]=[];
        for(const entry of entries){
            if(nodes.some(node=>node.id===entry.node.id))continue;
            for(const id of entry.assetVersionIds)assert(store.getAsset(id).projectId===projectId,'图片版本不属于此画布');
            nodes.push({...entry.node,selectedAssetVersionId:entry.node.selectedAssetVersionId||entry.displayAssetVersionId,position:findOpenPosition(nodes,entry.node.position)});restoredIds.push(entry.node.id);
            if(entry.legacy)store.db.prepare('INSERT OR IGNORE INTO canvas_trash VALUES (?,?,?,?,NULL)').run(entry.id,projectId,entry.node.id,JSON.stringify(entry));
        }
        const candidates=new Map<string,CanvasEdge>();
        // Restore retained links when both ends have returned, including sequential restores.
        const retained=(store.db.prepare('SELECT body FROM canvas_trash WHERE project_id=? ORDER BY rowid').all(projectId) as {body:string}[]).map(row=>JSON.parse(row.body) as TrashedCard);
        const latest=new Map([...retained,...entries].map(entry=>[entry.node.id,entry]));
        for(const entry of latest.values())for(const edge of entry.edges)if(restoredIds.includes(edge.sourceNodeId)||restoredIds.includes(edge.targetNodeId))candidates.set(edge.id,edge);
        let skippedEdges=0;
        for(const edge of candidates.values()){
            if(edges.some(existing=>existing.id===edge.id))continue;
            if(validateConnection({nodes,edges},edge).valid)edges.push(edge);else skippedEdges++;
        }
        const check=validateGraph({nodes,edges},store.listAssets(projectId));assert(check.valid,check.errors.join('；'));
        const next=store.patchGraph(projectId,project.graphRevision,{...project.graph,nodes,edges});
        return {project:next,restoredIds,skippedEdges};
    }).immediate();
}

export function importTrash(store:CanvasStore, entries:TrashedCard[]) {
    schema(store);
    for(const entry of entries)store.db.prepare('INSERT INTO canvas_trash VALUES (?,?,?,?,NULL)').run(entry.id,entry.projectId,entry.node.id,JSON.stringify(entry));
}
