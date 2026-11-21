import { createNode, DEFAULT_PARAMS } from '../src/domain/graph';
import type { Graph } from '../src/domain/types';
import fs from 'node:fs/promises';
async function main(){
const origin='http://127.0.0.1:3210';
const session=await fetch(origin+'/api/session');const cookie=session.headers.get('set-cookie')!.split(';')[0];const {csrfToken}=await session.json();
async function write(path:string,body:unknown,method='POST'){const r=await fetch(origin+'/api'+path,{method,headers:{Cookie:cookie,Origin:origin,'X-CSRF-Token':csrfToken,'Content-Type':'application/json'},body:JSON.stringify({...body as object,requestId:crypto.randomUUID()})});if(!r.ok)throw Error(await r.text());return r.json();}
const {project}=await write('/projects',{name:'验收临时画布 · 100 张图片',starter:false});
const nodes=Array.from({length:100},(_,i)=>{
 const node=createNode('generate',project.id,{x:(i%10)*290,y:Math.floor(i/10)*480});
 node.name=`验收图片 ${i+1}`;
 node.config={text:'本地画布性能验收，不运行生成。',params:{...DEFAULT_PARAMS}};
 return node;
});
const graph:Graph={nodes,edges:[],viewport:{x:30,y:30,zoom:.7}};
await write('/projects/'+project.id+'/graph',{expectedRevision:0,graph},'PATCH');
await fs.writeFile('artifacts/ui/benchmark-project.json',JSON.stringify({id:project.id,nodes:100,edges:0}));console.log({projectId:project.id,nodes:100,edges:0});
}main().catch(e=>{console.error(e);process.exitCode=1;});
