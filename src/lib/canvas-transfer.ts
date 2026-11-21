import { validateGraph } from '../domain/graph';
import type { CanvasClipboard } from './canvas-editing';

const PREFIX='CANVEPP_CLIPBOARD_V1\n';
export const encodeCanvasClipboard=(value:CanvasClipboard)=>PREFIX+JSON.stringify(value);
export function decodeCanvasClipboard(text:string):CanvasClipboard|undefined {
    if(!text.startsWith(PREFIX)||text.length>20*1024*1024)return;
    try {
        const value=JSON.parse(text.slice(PREFIX.length)) as CanvasClipboard;
        if(value?.version!==1||typeof value.projectId!=='string'||!Array.isArray(value.nodes)||!Array.isArray(value.edges)||!Array.isArray(value.incoming)||value.nodes.length>500||value.edges.length+value.incoming.length>1000)return;
        if(value.incoming.some(item=>!item?.source||!item.edge))return;
        const nodes=[...new Map([...value.nodes,...value.incoming.map(item=>item.source)].map(node=>[node.id,node])).values()];
        if(nodes.some(node=>node.projectId!==value.projectId)||!validateGraph({nodes,edges:[...value.edges,...value.incoming.map(item=>item.edge)]}).valid)return;
        return value;
    } catch { return; }
}
export function imageFiles(data:DataTransfer):File[] {
    const files=Array.from(data.files||[]);
    if(!files.length)for(const item of Array.from(data.items||[])){if(item.kind==='file'){const file=item.getAsFile();if(file)files.push(file);}}
    return files.filter(file=>/^image\/(png|jpeg|webp)$/.test(file.type)||!file.type&&/\.(png|jpe?g|webp)$/i.test(file.name));
}
export function transferImageUrls(data:DataTransfer):string[] {
    const urls=data.getData('text/uri-list').split(/\r?\n/).filter(line=>line&&!line.startsWith('#'));
    const html=data.getData('text/html');
    if(html){const doc=new DOMParser().parseFromString(html,'text/html');for(const img of doc.querySelectorAll('img[src]'))urls.push(img.getAttribute('src')!);}
    return [...new Set(urls)].filter(url=>/^https?:\/\//i.test(url)||/^data:image\/(png|jpeg|webp);base64,/i.test(url)).slice(0,20);
}
export async function fileFromImageUrl(url:string):Promise<File> {
    const response=await fetch(url,{credentials:'omit',referrerPolicy:'no-referrer',signal:AbortSignal.timeout(15000)});
    if(!response.ok||Number(response.headers.get('content-length')||0)>20*1024*1024)throw Error('无法读取这张图片');
    const reader=response.body?.getReader();if(!reader)throw Error('图片内容为空');const chunks:Uint8Array<ArrayBuffer>[]=[];let total=0;
    while(true){const {done,value}=await reader.read();if(done)break;total+=value.byteLength;if(total>20*1024*1024){await reader.cancel();throw Error('图片超过 20 MB');}chunks.push(value as Uint8Array<ArrayBuffer>);}
    const mime=response.headers.get('content-type')?.split(';')[0]||'image/png';if(!['image/png','image/jpeg','image/webp'].includes(mime))throw Error('请选择 PNG、JPEG 或 WebP 图片');
    return new File(chunks,`拖入图片.${mime==='image/jpeg'?'jpg':mime.split('/')[1]}`,{type:mime});
}
async function pngForClipboard(assetId:string):Promise<Blob> {
    const response=await fetch(`/api/assets/${encodeURIComponent(assetId)}`);if(!response.ok)throw Error('图片读取失败');const blob=await response.blob();if(blob.type==='image/png')return blob;
    const bitmap=await createImageBitmap(blob);try{const canvas=document.createElement('canvas');canvas.width=bitmap.width;canvas.height=bitmap.height;canvas.getContext('2d')!.drawImage(bitmap,0,0);return await new Promise<Blob>((resolve,reject)=>canvas.toBlob(result=>result?resolve(result):reject(Error('图片复制失败')),'image/png'));}finally{bitmap.close();}
}
export async function writeCanvasClipboard(value:CanvasClipboard,assetId?:string) {
    const text=encodeCanvasClipboard(value);
    if(assetId&&typeof ClipboardItem!=='undefined'&&navigator.clipboard?.write){
        await navigator.clipboard.write([new ClipboardItem({'text/plain':new Blob([text],{type:'text/plain'}),'image/png':pngForClipboard(assetId)})]);
    }else await navigator.clipboard.writeText(text);
}
export async function readCanvasClipboard():Promise<{canvas?:CanvasClipboard;files:File[]}> {
    if(!navigator.clipboard?.read){return{canvas:decodeCanvasClipboard(await navigator.clipboard.readText()),files:[]};}
    const items=await navigator.clipboard.read(),files:File[]=[];
    for(const item of items){if(item.types.includes('text/plain')){const canvas=decodeCanvasClipboard(await(await item.getType('text/plain')).text());if(canvas)return{canvas,files:[]};}for(const type of item.types.filter(type=>/^image\/(png|jpeg|webp)$/.test(type)))files.push(new File([await item.getType(type)],`粘贴图片.${type.split('/')[1]}`,{type}));}
    return {files};
}
