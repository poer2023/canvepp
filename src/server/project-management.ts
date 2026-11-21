import { unlinkSync, readdirSync } from 'node:fs';
import path from 'node:path';
import type { CanvasStore } from './store';
import { assert } from './errors';

/** Only called after a concrete UI confirmation. Exported backups are never touched. */
export function deleteProject(store:CanvasStore,projectId:string,input:{confirmed:boolean;expectedRevision:number}) {
 assert(input.confirmed===true,'删除项目需要明确确认');
 let runs: ReturnType<CanvasStore['listRuns']>=[];
 let files:{file:string}[]=[];
 store.db.transaction(()=>{
 const project=store.getProject(projectId);
 assert(project.graphRevision===input.expectedRevision,'项目已变更，请重新检查删除范围',409,'conflict');
 runs=store.listRuns(projectId);
 const pending=runs.filter(r=>['queued','submitting','running','awaiting_host','unknown'].includes(r.status));
 assert(pending.length===0,'项目仍有运行中或待对账任务，请先完成取消或对账',409,'conflict');
 // A locally cancelled request can still produce a late result. Its reservation
 // remains held until the worker settles, and deletion must not race that worker.
 for(const run of runs){
  const ledger=store.db.prepare('SELECT state FROM budget WHERE run_id=?').get(run.id) as {state:string}|undefined;
  assert(!(run.status==='cancelled'&&ledger?.state==='reserved'),'取消的任务仍等待供应商结果，请稍后再删除',409,'conflict');
 }
 files=store.db.prepare('SELECT DISTINCT file FROM assets WHERE project_id=?').all(projectId) as {file:string}[];
  for(const run of runs){
   const ledger=store.db.prepare('SELECT period,state,actual FROM budget WHERE run_id=?').get(run.id) as {period:string;state:string;actual:number|null}|undefined;
   if(ledger?.state==='settled_estimate'&&(ledger.actual??0)>0){
    // Keep anonymous monthly totals so deleting a project cannot reset a budget.
    store.db.prepare("INSERT INTO budget(run_id,period,reserved,actual,state) VALUES(?,?,0,?,'settled_estimate') ON CONFLICT(run_id) DO UPDATE SET actual=actual+excluded.actual").run(`deleted:${ledger.period}`,ledger.period,ledger.actual);
   }
   store.db.prepare('DELETE FROM host_claims WHERE run_id=?').run(run.id);
   if(store.db.prepare("SELECT 1 FROM sqlite_master WHERE type='table' AND name='host_dispatches'").get())store.db.prepare('DELETE FROM host_dispatches WHERE run_id=?').run(run.id);
   store.db.prepare('DELETE FROM confirmations WHERE run_id=?').run(run.id);
   store.db.prepare('DELETE FROM jobs WHERE run_id=?').run(run.id);
   store.db.prepare('DELETE FROM budget WHERE run_id=?').run(run.id);
  }
  const previews=store.db.prepare('SELECT id,body FROM previews').all() as {id:string;body:string}[];
  for(const preview of previews)if(JSON.parse(preview.body).projectId===projectId)store.db.prepare('DELETE FROM previews WHERE id=?').run(preview.id);
  store.db.prepare('DELETE FROM runs WHERE project_id=?').run(projectId);
  store.db.prepare('DELETE FROM assets WHERE project_id=?').run(projectId);
  if(store.db.prepare("SELECT 1 FROM sqlite_master WHERE type='table' AND name='canvas_trash'").get())store.db.prepare('DELETE FROM canvas_trash WHERE project_id=?').run(projectId);
  store.db.prepare('DELETE FROM projects WHERE id=?').run(projectId);
 }).immediate();
 let retainedFiles=0;
 for(const {file} of files){
  if(store.db.prepare('SELECT 1 FROM assets WHERE file=? LIMIT 1').get(file))continue;
  try{unlinkSync(path.join(store.dataDir,'assets',file));}catch(error){if((error as NodeJS.ErrnoException).code!=='ENOENT')retainedFiles++;}
  try{unlinkSync(path.join(store.dataDir,'thumbnails',file.split('.')[0]+'.webp'));}catch(error){if((error as NodeJS.ErrnoException).code!=='ENOENT')retainedFiles++;}
 }
 const runIds=new Set(runs.map(r=>r.id));
 for(const file of readdirSync(path.join(store.dataDir,'staging'))){
  if(![...runIds].some(runId=>file.startsWith(runId+'-')||file===runId+'.json'))continue;
  try{unlinkSync(path.join(store.dataDir,'staging',file));}catch{retainedFiles++;}
 }
 return {deleted:true,projectId,retainedFiles,backupsDeleted:false};
}
