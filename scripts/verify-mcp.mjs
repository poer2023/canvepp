import { Client } from '@modelcontextprotocol/sdk/client/index.js';
import { StdioClientTransport } from '@modelcontextprotocol/sdk/client/stdio.js';
import assert from 'node:assert/strict';
import path from 'node:path';
const client = new Client({name:'canvepp-protocol-test',version:'0.1.0'});
const transport = new StdioClientTransport({command:process.execPath,args:[path.join(import.meta.dirname,'mcp-server.mjs')],env:{...process.env,CANVEPP_URL:process.env.CANVEPP_URL||'http://127.0.0.1:3210'},stderr:'pipe'});
await client.connect(transport);
try {
 const tools=await client.listTools();
 assert.equal(tools.tools.length,8);
 assert(tools.tools.filter(t=>['canvepp_list_projects','canvepp_project_status','canvepp_run_status','canvepp_pending_runs'].includes(t.name)).every(t=>t.annotations?.readOnlyHint===true));
 assert(tools.tools.find(t=>t.name==='canvepp_open_canvas').annotations.readOnlyHint===false);
 assert(tools.tools.find(t=>t.name==='canvepp_claim_native_run').annotations.readOnlyHint===false);
 const response=await client.callTool({name:'canvepp_open_canvas',arguments:{threadId:process.env.CODEX_THREAD_ID}});
 assert.equal(response.isError,undefined);
 assert.equal(response.structuredContent.generationSubmitted,false);
 let projectReads=0,runReads=0;
 if(response.structuredContent.available){
  const listing=await client.callTool({name:'canvepp_list_projects',arguments:{}});assert(!listing.isError);
  for(const project of listing.structuredContent.projects.slice(0,3)){
   const detail=await client.callTool({name:'canvepp_project_status',arguments:{projectId:project.id}});assert(!detail.isError);projectReads++;
   if(detail.structuredContent.runs.length){const run=await client.callTool({name:'canvepp_run_status',arguments:{runId:detail.structuredContent.runs[0].runId}});assert(!run.isError);assert.equal(run.structuredContent.generationSubmitted,false);runReads++;break;}
  }
 }
 const resources=await client.listResources();
 assert(resources.resources.some(r=>r.uri==='canvepp://help'));
 await client.readResource({uri:'canvepp://help'});
 console.log(JSON.stringify({ok:true,toolCount:tools.tools.length,resourceCount:resources.resources.length,serviceAvailable:response.structuredContent.available,projectReads,runReads,externalGenerationRequests:0}));
} finally {await client.close();}
