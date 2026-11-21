import {hostRequestBody} from '@/server/host-request';
import {getStore} from '@/server/store';
import {authorize,checkOrigin} from '@/server/security';
import {assert,publicError} from '@/server/errors';
import {readHostContext,registerHostContext} from '@/server/host-dispatch';
export const runtime='nodejs';export const dynamic='force-dynamic';
const headers={'Cache-Control':'no-store','X-Content-Type-Options':'nosniff'};
export async function GET(request:Request){try{checkOrigin(request);const store=getStore();authorize(store,request);return Response.json(readHostContext(store,request.headers.get('x-canvepp-host')??undefined),{headers});}catch(error){const e=publicError(error);return Response.json(e,{status:e.status,headers});}}
export async function POST(request:Request){try{checkOrigin(request);const store=getStore();authorize(store,request);const body=await hostRequestBody(request);assert(typeof body.requestId==='string'&&typeof body.threadId==='string','缺少 requestId 或 threadId');return Response.json(registerHostContext(store,body.threadId,typeof body.label==='string'?body.label:undefined),{headers});}catch(error){const e=publicError(error);return Response.json(e,{status:e.status,headers});}}
