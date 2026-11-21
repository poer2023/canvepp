import { describe, expect, it } from 'vitest';
import { copySelection, pasteSelection } from '../src/lib/canvas-editing';
import { encodeCanvasClipboard, decodeCanvasClipboard } from '../src/lib/canvas-transfer';
import { createNode } from '../src/domain/graph';
describe('OS clipboard canvas snapshots',()=>{
 it('round trips complete multi-selection and links without changing original data',()=>{
  const a=createNode('image','p'),b=createNode('generate','p',{x:400,y:30});a.selectedAssetVersionId='asset-original';b.config.text='保留这些细节';b.config.note='备注';
  const graph={nodes:[a,b],edges:[{id:'link',sourceNodeId:a.id,sourcePort:'out',targetNodeId:b.id,targetPort:'reference'}]},before=structuredClone(graph);
  const copied=copySelection(graph,'p',[a.id,b.id])!,decoded=decodeCanvasClipboard(encodeCanvasClipboard(copied));expect(decoded).toEqual(copied);
  const pasted=pasteSelection(graph,decoded,{projectId:'p'});expect(pasted.graph.nodes[2].selectedAssetVersionId).toBe('asset-original');expect(pasted.graph.nodes[3].config).toEqual(b.config);expect(pasted.graph.edges[1].targetNodeId).toBe(pasted.nodeIds[1]);expect(graph).toEqual(before);
 });
 it('retains an unselected input source captured with a card',()=>{
  const a=createNode('image','p'),b=createNode('generate','p');a.selectedAssetVersionId='source';const graph={nodes:[a,b],edges:[{id:'link',sourceNodeId:a.id,sourcePort:'out',targetNodeId:b.id,targetPort:'reference'}]};const data=copySelection(graph,'p',[b.id])!;
  expect(decodeCanvasClipboard(encodeCanvasClipboard(data))?.incoming[0].source).toEqual(a);
 });
 it('ignores ordinary text, oversized and malformed snapshots',()=>{
  expect(decodeCanvasClipboard('日常文本')).toBeUndefined();expect(decodeCanvasClipboard('CANVEPP_CLIPBOARD_V1\n{')).toBeUndefined();expect(decodeCanvasClipboard('CANVEPP_CLIPBOARD_V1\n'+'x'.repeat(20*1024*1024))).toBeUndefined();
  const data=copySelection({nodes:[createNode('image','p')],edges:[]},'p',[])!;expect(data).toBeUndefined();
  const invalid={version:1,projectId:'p',nodes:[{id:'x',projectId:'p',kind:'image',position:{x:NaN,y:0}}],edges:[],incoming:[]};expect(decodeCanvasClipboard('CANVEPP_CLIPBOARD_V1\n'+JSON.stringify(invalid))).toBeUndefined();
 });
});
