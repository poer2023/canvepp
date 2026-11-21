import { afterEach, describe, expect, it } from 'vitest';
import { mkdtempSync, rmSync, existsSync } from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { CanvasStore } from '../src/server/store';
import { deleteProject } from '../src/server/project-management';
const resources:{store:CanvasStore;dir:string}[]=[];
function make(){const dir=mkdtempSync(path.join(os.tmpdir(),'canvepp-delete-'));const store=new CanvasStore(dir,{autoRun:false});resources.push({store,dir});return store;}
afterEach(()=>{for(const {store,dir} of resources.splice(0)){store.close();rmSync(dir,{recursive:true,force:true});}});
describe('explicit local project deletion',()=>{
 it('requires confirmation and the current revision',async()=>{const s=make();const {project:p}=await s.createProject('delete checks');expect(()=>deleteProject(s,p.id,{confirmed:false,expectedRevision:0})).toThrow();expect(()=>deleteProject(s,p.id,{confirmed:true,expectedRevision:99})).toThrow();expect(s.listProjects()).toHaveLength(1);expect(deleteProject(s,p.id,{confirmed:true,expectedRevision:0}).deleted).toBe(true);expect(s.listProjects()).toHaveLength(0);});
 it('preserves shared immutable content referenced by another project',async()=>{const s=make();const first=await s.createProject('first',true);const second=await s.createProject('second',true);expect(first.assets[0].sha256).toBe(second.assets[0].sha256);deleteProject(s,first.project.id,{confirmed:true,expectedRevision:first.project.graphRevision});expect(s.assetBytes(second.assets[0].id).byteLength).toBeGreaterThan(0);const row=s.db.prepare('SELECT file FROM assets WHERE id=?').get(second.assets[0].id) as {file:string};deleteProject(s,second.project.id,{confirmed:true,expectedRevision:second.project.graphRevision});expect(existsSync(path.join(s.dataDir,'assets',row.file))).toBe(false);});
 it('blocks queued and unknown tasks and keeps anonymous spent amounts',async()=>{const s=make();const d=await s.createProject('run history',true);const node=d.project.graph.nodes.find(n=>n.kind==='edit')!;const preview=s.preview(d.project.id,node.id,d.project.graphRevision,'mock');const run=s.confirm({previewId:preview.id,snapshotHash:preview.snapshotHash,confirmationId:'delete-confirm',idempotencyKey:'delete-key'});expect(()=>deleteProject(s,d.project.id,{confirmed:true,expectedRevision:0})).toThrow(/任务/);s.cancel(run.id);s.db.prepare("UPDATE budget SET state='settled_estimate',actual=2 WHERE run_id=?").run(run.id);deleteProject(s,d.project.id,{confirmed:true,expectedRevision:0});expect(s.budget().spent).toBe(2);expect(s.db.prepare('SELECT 1 FROM runs WHERE id=?').get(run.id)).toBeUndefined();});
});
