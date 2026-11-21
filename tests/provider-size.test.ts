import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { mkdtempSync, rmSync } from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import sharp from 'sharp';
import type { ImageSize, InputSnapshot, RunPreview } from '../src/domain/types';
import { MockProvider, OpenAIProvider, OPENAI_CAPABILITIES, mockImage, validateProviderInput } from '../src/server/providers';
import { CanvasStore } from '../src/server/store';
let dir:string,store:CanvasStore;
const snapshot=(provider:string,size:ImageSize='713x509',n=1):InputSnapshot=>({nodeId:'fixture',nodeKind:'generate',configVersion:1,prompt:'一只放在浅灰桌面上的白色陶瓷杯',params:{size,quality:'high',n},upstreamStale:[],provider,model:provider==='openai'?'gpt-image-1.5':provider==='codex'?'codex-native-imagegen':provider});
const confirm=(preview:RunPreview)=>store.confirm({previewId:preview.id,snapshotHash:preview.snapshotHash,idempotencyKey:crypto.randomUUID(),confirmationId:crypto.randomUUID()});
beforeEach(()=>{dir=mkdtempSync(path.join(os.tmpdir(),'canvepp-provider-size-'));store=new CanvasStore(dir,{autoRun:false,mockDelayMs:0});vi.stubGlobal('fetch',vi.fn(()=>{throw Error('Unexpected outbound request in isolated size fixture');}));});
afterEach(()=>{store.close();rmSync(dir,{recursive:true,force:true});vi.unstubAllGlobals();});
async function fixture(size:ImageSize='713x509',n=1){const created=await store.createProject('尺寸意图测试',false,'image'),node=created.project.graph.nodes[0];const graph=structuredClone(created.project.graph);graph.nodes[0].config={text:'保持主体构图，换成浅灰背景',params:{size,quality:'high',n}};const project=store.patchGraph(created.project.id,0,graph);return {project,node};}

describe('provider dimension boundaries and immutable size intent',()=>{
 it.each(['mock','manual','codex'])('accepts legal custom dimensions in %s and retains compatible quality metadata',provider=>{
  expect(()=>validateProviderInput(snapshot(provider))).not.toThrow();expect(()=>validateProviderInput(snapshot(provider,'64x64'))).not.toThrow();expect(()=>validateProviderInput(snapshot(provider,'4000x8000'))).not.toThrow();expect(()=>validateProviderInput(snapshot(provider,'8192x8192'))).toThrow('3200 万');expect(fetch).not.toHaveBeenCalled();
 });
 it('keeps native tasks at one image while manual and mock retain their existing batch allowance',()=>{
  expect(()=>validateProviderInput(snapshot('codex','713x509',2))).toThrow('仅支持 1 张');expect(()=>validateProviderInput(snapshot('manual','713x509',4))).not.toThrow();expect(()=>validateProviderInput(snapshot('mock','713x509',4))).not.toThrow();
 });
 it('renders actual bounded custom PNGs for mock generation and editing without normalizing the target size',async()=>{
  const provider=new MockProvider(),generated=await provider.submit(snapshot('mock','173x97',2));expect(generated.images).toHaveLength(2);for(const bytes of generated.images){const meta=await sharp(bytes).metadata();expect([meta.width,meta.height,meta.format]).toEqual([173,97,'png']);}
  const source=await sharp({create:{width:20,height:30,channels:3,background:'white'}}).png().toBuffer();const edited=await provider.submit({...snapshot('mock','97x173'),nodeKind:'edit'},source);const meta=await sharp(edited.images[0]).metadata();expect([meta.width,meta.height]).toEqual([97,173]);const minimum=await sharp(await mockImage(0,undefined,'64x64')).metadata();expect([minimum.width,minimum.height]).toEqual([64,64]);expect(fetch).not.toHaveBeenCalled();
 });
 it('guards the direct mock renderer before a caller can allocate oversized or malformed output',async()=>{
  await expect(mockImage(0,undefined,'8192x8192')).rejects.toThrow('像素限制');await expect(mockImage(0,undefined,'100.5x300')).rejects.toThrow('整数宽x高');expect(fetch).not.toHaveBeenCalled();
 });
 it('keeps independent API sizes fixed and rejects a forged non-API snapshot before any request',async()=>{
  expect(OPENAI_CAPABILITIES.sizes).toEqual(['1024x1024','1536x1024','1024x1536']);for(const size of OPENAI_CAPABILITIES.sizes)expect(()=>validateProviderInput(snapshot('openai',size))).not.toThrow();await expect(new OpenAIProvider().submit(snapshot('openai'))).rejects.toThrow('已配置的图片尺寸');await expect(new OpenAIProvider().submit({...snapshot('manual'),model:'gpt-image-1.5'})).rejects.toThrow('供应商快照');expect(fetch).not.toHaveBeenCalled();
 });
 it('preserves custom native size through preview, confirmation and claim without using an API worker',async()=>{
  const {project,node}=await fixture(),preview=store.preview(project.id,node.id,project.graphRevision,'codex');expect(preview.snapshot.params).toEqual({size:'713x509',quality:'high',n:1});const run=confirm(preview),claim=store.claimHostRun(run.id,{claimId:'custom-size',clientId:'fixture-client'},'fixture-local-session');expect(claim.canGenerate).toBe(true);expect(claim.run.inputSnapshot).toEqual(preview.snapshot);expect(claim.run.inputSnapshot.params.size).toBe('713x509');expect((await store.processRun(run.id)).status).toBe('running');expect(store.listAssets(project.id)).toEqual([]);expect(fetch).not.toHaveBeenCalled();
 });
 it('stores custom manual intent while API custom dimensions and unknown total pricing remain blocked',async()=>{
  const {project,node}=await fixture('713x509',2),manual=confirm(store.preview(project.id,node.id,project.graphRevision,'manual'));expect(manual.status).toBe('awaiting_import');expect(manual.inputSnapshot.params).toEqual({size:'713x509',quality:'high',n:2});store.saveSettings({...store.budget().settings,apiEnabled:true});expect(()=>store.preview(project.id,node.id,project.graphRevision,'api')).toThrow('已配置的图片尺寸');const graph=structuredClone(project.graph);graph.nodes[0].config.params!.size='1024x1024';const saved=store.patchGraph(project.id,project.graphRevision,graph);try{store.preview(saved.id,node.id,saved.graphRevision,'api');throw Error('Expected the unchanged API cost gate to refuse');}catch(error){expect(error).toMatchObject({errorClass:'quota'});}expect(store.listRuns(project.id)).toHaveLength(1);expect(fetch).not.toHaveBeenCalled();
 });
});
