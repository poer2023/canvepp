import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import sharp from 'sharp';
import { OpenAIProvider } from '../src/server/providers';
import type { InputSnapshot } from '../src/domain/types';
const snapshot:InputSnapshot={nodeId:'node',nodeKind:'generate',configVersion:1,prompt:'a ceramic cup',params:{size:'1024x1024',quality:'low',n:1},upstreamStale:[],provider:'openai',model:'gpt-image-1.5',providerOrganization:'org-confirmed',providerProject:'proj_confirmed'};
let originalKey:string|undefined;
beforeEach(()=>{originalKey=process.env.OPENAI_API_KEY;process.env.OPENAI_API_KEY='local-test-credential';});
afterEach(()=>{vi.unstubAllGlobals();if(originalKey===undefined)delete process.env.OPENAI_API_KEY;else process.env.OPENAI_API_KEY=originalKey;});
describe('OpenAI adapter contract with offline fetch stubs',()=>{
 it('submits the frozen account and the supported request parameters exactly once',async()=>{
  const png=await sharp({create:{width:2,height:2,channels:3,background:'red'}}).png().toBuffer();const fetchMock=vi.fn(async(_input:RequestInfo|URL,_init?:RequestInit)=>new Response(JSON.stringify({data:[{b64_json:png.toString('base64')}],usage:{output_tokens:42}}),{headers:{'x-request-id':'req-confirmed'}}));vi.stubGlobal('fetch',fetchMock);
  const result=await new OpenAIProvider().submit(snapshot);expect(fetchMock).toHaveBeenCalledTimes(1);expect(fetchMock.mock.calls[0][0]).toBe('https://api.openai.com/v1/images/generations');const options=fetchMock.mock.calls[0][1] as RequestInit;expect(options.headers).toMatchObject({'OpenAI-Organization':'org-confirmed','OpenAI-Project':'proj_confirmed'});expect(JSON.parse(options.body as string)).toMatchObject({model:'gpt-image-1.5',n:1,size:'1024x1024',quality:'low',output_format:'png'});expect(result.providerRequestId).toBe('req-confirmed');expect(result.images[0]).toEqual(png);
 });
 it('normalizes a preserved JPEG reference to a real PNG multipart part',async()=>{
  const jpeg=await sharp({create:{width:3,height:4,channels:3,background:'green'}}).jpeg().toBuffer();const fetchMock=vi.fn(async(_input:RequestInfo|URL,_init?:RequestInit)=>new Response(JSON.stringify({data:[{b64_json:jpeg.toString('base64')}]})));vi.stubGlobal('fetch',fetchMock);await new OpenAIProvider().submit({...snapshot,nodeKind:'edit'},jpeg);const options=fetchMock.mock.calls[0][1] as RequestInit;const part=(options.body as FormData).get('image') as File;expect(part.type).toBe('image/png');expect((await sharp(Buffer.from(await part.arrayBuffer())).metadata()).format).toBe('png');
 });
 it.each([{status:429,code:'insufficient_quota',expected:'quota'},{status:429,code:'rate_limit_exceeded',expected:'rate_limit'},{status:400,code:'content_policy_violation',expected:'policy'},{status:401,code:'invalid_api_key',expected:'authorization'},{status:500,code:'server_error',expected:'transport_unknown'}])('classifies $code without leaking provider error text or retrying',async({status,code,expected})=>{
  const fetchMock=vi.fn(async(_input:RequestInfo|URL,_init?:RequestInit)=>new Response(JSON.stringify({error:{code,message:'sensitive-provider-text local-test-credential'}}),{status,headers:{'x-request-id':'req-failed'}}));vi.stubGlobal('fetch',fetchMock);const error=await new OpenAIProvider().submit(snapshot).catch(e=>e);expect(error.errorClass).toBe(expected);expect(error.providerRequestId).toBe('req-failed');expect(error.message).not.toContain('sensitive-provider-text');expect(error.message).not.toContain('local-test-credential');expect(fetchMock).toHaveBeenCalledTimes(1);
 });
 it('network interruption is unknown, and never triggers an automatic retry',async()=>{
  const fetchMock=vi.fn(async()=>{throw new Error('socket failed');});vi.stubGlobal('fetch',fetchMock);await expect(new OpenAIProvider().submit(snapshot)).rejects.toMatchObject({errorClass:'transport_unknown'});expect(fetchMock).toHaveBeenCalledTimes(1);
 });
 it('missing server key stops before any outbound request',async()=>{
  delete process.env.OPENAI_API_KEY;const fetchMock=vi.fn();vi.stubGlobal('fetch',fetchMock);await expect(new OpenAIProvider().submit(snapshot)).rejects.toMatchObject({errorClass:'authorization'});expect(fetchMock).not.toHaveBeenCalled();
 });
});
