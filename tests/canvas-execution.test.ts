import { randomUUID } from 'node:crypto';
import { mkdtempSync, rmSync } from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import sharp from 'sharp';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import type { ImageParams } from '../src/domain/types';
import { createNode, DEFAULT_PARAMS } from '../src/domain/graph';
import { runCanvasExecutions } from '../src/server/canvas-execution';
import type { NativeNotice, NativeTurn } from '../src/server/codex-app-server';
import { confirmGenerationGroup, previewGenerationGroup } from '../src/server/generation-groups';
import { CanvasStore } from '../src/server/store';

let dir: string, store: CanvasStore, image: Buffer;
beforeEach(async () => {
    dir = mkdtempSync(path.join(os.tmpdir(), 'canvepp-native-'));
    store = new CanvasStore(dir, {autoRun:false});
    image = await sharp({create:{width:64,height:64,channels:4,background:'#344860'}}).png().toBuffer();
});
afterEach(() => { store.close(); rmSync(dir,{recursive:true,force:true}); });

async function confirmed(count = 1, referenceCount = 0) {
    const {project} = await store.createProject('测试画布');
    const params: ImageParams = {...DEFAULT_PARAMS, size:'128x128', n:count};
    const nodes = [];
    for (let i = 0; i < Math.max(referenceCount,1); i++) {
        const node = createNode('generate', project.id, {x:0,y:i*420});
        node.config = {text:'保持参考图顺序',params};
        if (referenceCount) node.selectedAssetVersionId = (await store.importAsset(image,project.id,'参考图 '+i+'.png')).id;
        nodes.push(node);
    }
    const graph = store.patchGraph(project.id,project.graphRevision,{nodes,edges:[]});
    const preview = previewGenerationGroup(store,{projectId:project.id,graphRevision:graph.graphRevision,mode:'codex',
        execution:'canvas',sourceNodeIds:nodes.map(n=>n.id).reverse(),prompt:'组合参考图',params});
    const confirmation = {previewId:preview.id,snapshotHash:preview.snapshotHash,confirmationKey:randomUUID(),acceptStale:!!referenceCount};
    const group = confirmGenerationGroup(store,confirmation);
    expect(confirmGenerationGroup(store,confirmation).runs.map(r=>r.id)).toEqual(group.runs.map(r=>r.id));
    return {group, preview, nodes};
}

class TestHost {
    readonly thread = randomUUID();
    readonly turns: NativeTurn[] = [];
    readonly calls: string[] = [];
    readonly referencePaths: string[][] = [];
    disconnectOnce = false;
    failAccount = false;
    provider = 'openai';
    onStart?: () => void;
    onNotice?: (method:string,notice:NativeNotice) => void;
    onDisconnect?: () => void;
    async initialize() {}
    async requireNativeAccount() { if (this.failAccount) throw new Error('请先登录 Codex'); }
    close() {}
    async call<T>(method:string, params:unknown):Promise<T> {
        this.calls.push(method);
        if (method==='thread/start' || method==='thread/resume' || method==='thread/read')
            return {thread:{id:this.thread,status:{type:'idle'}},modelProvider:this.provider} as T;
        if (method==='thread/name/set') return {} as T;
        if (method==='thread/turns/list') return {data:this.turns,nextCursor:null} as T;
        if (method!=='turn/start') throw new Error('意外的方法 '+method);
        const {input} = params as {input:{type:string,text?:string,path?:string}[]};
        this.referencePaths.push(input.flatMap(i=>i.type==='localImage' && i.path ? [i.path] : []));
        const turn:NativeTurn = {id:randomUUID(),status:'completed',items:[
            {type:'userMessage',id:randomUUID(),content:input.filter(i=>i.type==='text')},
            {type:'imageGeneration',id:randomUUID(),result:image.toString('base64')},
        ]};
        this.turns.push(turn);
        this.onStart?.();
        if (this.disconnectOnce) {
            this.disconnectOnce=false;
            this.onDisconnect?.();
            throw new Error('受理后连接中断');
        }
        // Fast native notifications before the start acknowledgment.
        this.onNotice?.('item/completed',{threadId:this.thread,turnId:turn.id,item:turn.items[1]});
        this.onNotice?.('turn/completed',{threadId:this.thread,turn});
        return {turn} as T;
    }
}

describe('independent native canvas execution', () => {
    it('reuses the canvas chat and fixes every ordered reference in each single-result turn', async () => {
        const {group,preview}=await confirmed(2,2);
        const host=new TestHost();
        await Promise.all([runCanvasExecutions(store,()=>host),runCanvasExecutions(store,()=>host)]);
        expect(host.calls.filter(m=>m==='thread/start')).toHaveLength(1);
        expect(host.calls.filter(m=>m==='thread/resume')).toHaveLength(1);
        expect(host.turns).toHaveLength(2);
        const files=preview.references.map(asset=>{
            const row=store.db.prepare('SELECT file FROM assets WHERE id=?').get(asset.id) as {file:string};
            return path.join(dir,'assets',row.file);
        });
        expect(host.referencePaths).toEqual([files,files]);
        for (const [index, child] of group.runs.entries()) {
            const run=store.getRun(child.id);
            expect(run.status).toBe('succeeded');
            expect(run.usage?.canvasExecution).toEqual({threadId:host.thread,turnId:host.turns[index].id});
            expect(store.getProject(run.projectId).graph.nodes.find(n=>n.id===run.nodeId)?.selectedAssetVersionId).toBe(run.assetVersionIds[0]);
        }
    });
    it('reconciles a lost acknowledgment after reopening the store, using only the confirmed turn marker', async () => {
        const {group}=await confirmed();
        const host=new TestHost(); host.disconnectOnce=true;
        await runCanvasExecutions(store,()=>host);
        const id=group.runs[0].id;
        expect(store.getRun(id).status).toBe('unknown');
        expect(store.getRun(id).usage?.canvasExecution).toEqual({threadId:host.thread});
        store.close(); store=new CanvasStore(dir,{autoRun:false});
        store.db.prepare('UPDATE canvas_native_jobs SET next_check=? WHERE run_id=?').run(Date.now(),id);
        await runCanvasExecutions(store,()=>host);
        await runCanvasExecutions(store,()=>host);
        expect(host.turns).toHaveLength(1);
        expect(host.calls.filter(m=>m==='thread/turns/list')).toHaveLength(1);
        expect(store.getRun(id).usage?.canvasExecution).toEqual({threadId:host.thread,turnId:host.turns[0].id});
        expect(store.getRun(id).status).toBe('succeeded');
        expect(store.listAssets(group.project.id)).toHaveLength(1);
    });
    it('preserves a cancelled child’s late result in its history without adopting it', async () => {
        const {group}=await confirmed();
        const host=new TestHost(), id=group.runs[0].id;
        host.onStart=()=>{store.cancel(id);};
        await runCanvasExecutions(store,()=>host);
        const run=store.getRun(id);
        expect(run.lateResult).toBe(true);
        expect(run.assetVersionIds).toHaveLength(1);
        expect(store.getProject(run.projectId).graph.nodes.find(n=>n.id===run.nodeId)?.selectedAssetVersionId).toBeUndefined();
    });
    it('restores local history without scheduling the imported receipt or blocking new work on the copy', async () => {
        const {group} = await confirmed();
        const host = new TestHost();
        host.disconnectOnce = true;
        await runCanvasExecutions(store, () => host);
        const copy = await store.importProject(await store.exportProject(group.project.id));
        const old = copy.runs[0];
        expect(old.historyOnly).toBe(true);
        expect(old.usage?.canvasExecution).toEqual({threadId:host.thread});
        expect(store.db.prepare('SELECT 1 FROM canvas_native_jobs WHERE run_id=?').get(old.id)).toBeUndefined();
        const source = (old.usage?.generationGroup as {sourceNodeIds:string[]}).sourceNodeIds[0];
        const fresh = previewGenerationGroup(store, {projectId:copy.project.id, graphRevision:copy.project.graphRevision, mode:'codex',execution:'canvas',sourceNodeIds:[source]});
        expect(fresh.nodes).toHaveLength(1);
        expect(host.turns).toHaveLength(1);
    });
    it.each(['signed out','other provider','cancelled before claim'])('does not submit a native turn when %s', async reason => {
        const {group}=await confirmed();
        const host=new TestHost(), id=group.runs[0].id;
        host.failAccount=reason==='signed out';
        if (reason==='other provider') host.provider='other';
        if (reason==='cancelled before claim') store.cancel(id);
        await runCanvasExecutions(store,()=>host);
        expect(store.getRun(id).status).toBe(reason==='cancelled before claim'?'cancelled':'failed');
        expect(host.turns).toHaveLength(0);
        expect(store.listAssets(group.project.id)).toHaveLength(0);
    });
});
