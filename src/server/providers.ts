import sharp from 'sharp';
import type { InputSnapshot, ProviderCapabilities } from '../domain/types';
import { AppError, assert } from './errors';
import { IMAGE_SIZE_LIMITS, parseImageSize, validateImageSize } from '../domain/image-size';
export const OPENAI_CAPABILITIES: ProviderCapabilities = {provider:'openai',model:'gpt-image-1.5',sizes:['1024x1024','1536x1024','1024x1536'],qualities:['low','medium','high'],maxImages:4,query:false,cancel:false};
export interface ProviderResult { images: Buffer[]; providerRequestId?:string; usage?:Record<string,unknown>; }
export interface ImageProvider { capabilities:ProviderCapabilities; submit(snapshot:InputSnapshot, source?:Buffer):Promise<ProviderResult>; }
export function validateProviderInput(snapshot: InputSnapshot) {
 const references=snapshot.sourceAssetVersionIds;
 if(references!==undefined){assert(Array.isArray(references)&&references.length>=1&&references.length<=8&&references.every(id=>typeof id==='string'&&id.length>0),'参考图片列表无效');assert(Array.isArray(snapshot.sourceAssetHashes)&&snapshot.sourceAssetHashes.length===references.length&&snapshot.sourceAssetHashes.every(hash=>/^[a-f0-9]{64}$/i.test(hash)),'参考图片校验信息无效');assert(snapshot.sourceAssetVersionId===references[0]&&snapshot.sourceAssetHash===snapshot.sourceAssetHashes[0],'主图与参考顺序不一致');}
 if(snapshot.provider==='openai')assert(!references||references.length<=1,'当前独立 API 不支持多图参考');
 const sizeErrors=validateImageSize(snapshot.params.size);assert(sizeErrors.length===0,sizeErrors.join('；'));
 // The independent API adapter keeps its reviewed fixed sizes and pricing gate.
 if(snapshot.provider==='openai')assert(OPENAI_CAPABILITIES.sizes.includes(snapshot.params.size),'独立 API 仅支持当前适配器已配置的图片尺寸');
 assert(OPENAI_CAPABILITIES.qualities.includes(snapshot.params.quality),'不支持的质量');assert(Number.isInteger(snapshot.params.n)&&snapshot.params.n>=1&&snapshot.params.n<=4,'每次仅支持 1–4 张');
 if(snapshot.provider==='codex')assert(snapshot.params.n===1,'Codex 原生模式首版每次仅支持 1 张');assert(snapshot.prompt.trim().length>0&&snapshot.prompt.length<=32000,'提示词长度须为 1–32000 字符');
}
const palette = ['#65826f','#a88c6b','#7e8f9a','#8d7d95'];
export async function mockImage(index=0, source?:Buffer, size='1024x1024'):Promise<Buffer> {
 const dimensions=parseImageSize(size);assert(dimensions,'图片尺寸须为合法的整数宽x高，且不超过单图像素限制');const {width,height}=dimensions;const color=palette[index%palette.length];
 let image:sharp.Sharp;
 if(source) image=sharp(source,{limitInputPixels:40_000_000}).resize(width,height,{fit:'contain',background:'#ece9e1'}).modulate({brightness:1.04,saturation:.85+(index%3)*.15});
 else {
 const svg=`<svg width="${width}" height="${height}" xmlns="http://www.w3.org/2000/svg"><defs><radialGradient id="bg"><stop stop-color="#f0ede4"/><stop offset="1" stop-color="#d8d7cc"/></radialGradient><filter id="s"><feGaussianBlur stdDeviation="13"/></filter></defs><rect width="100%" height="100%" fill="url(#bg)"/><ellipse cx="${width*.51}" cy="${height*.56}" rx="${width*.34}" ry="${height*.24}" fill="#777d67" opacity=".15" filter="url(#s)"/><ellipse cx="${width*.5}" cy="${height*.5}" rx="${width*.34}" ry="${height*.27}" fill="#fbfaf6" stroke="#e5e3da" stroke-width="9"/><ellipse cx="${width*.5}" cy="${height*.5}" rx="${width*.26}" ry="${height*.2}" fill="#e6e2d5"/><path d="M ${width*.29} ${height*.5} Q ${width*.48} ${height*.25} ${width*.7} ${height*.45} Q ${width*.7} ${height*.7} ${width*.4} ${height*.63}Z" fill="${color}"/><g fill="#b9c994"><ellipse cx="${width*.44}" cy="${height*.44}" rx="${width*.1}" ry="${height*.04}" transform="rotate(-28 ${width*.44} ${height*.44})"/><ellipse cx="${width*.55}" cy="${height*.57}" rx="${width*.11}" ry="${height*.035}" transform="rotate(25 ${width*.55} ${height*.57})"/></g><g fill="#dcac69"><circle cx="${width*.47}" cy="${height*.48}" r="${width*.035}"/><circle cx="${width*.58}" cy="${height*.45}" r="${width*.027}"/><circle cx="${width*.4}" cy="${height*.55}" r="${width*.022}"/></g><path d="M ${width*.13} ${height*.24} L ${width*.17} ${height*.8}" stroke="#8e9686" stroke-width="12" stroke-linecap="round"/></svg>`;
 image=sharp(Buffer.from(svg),{limitInputPixels:IMAGE_SIZE_LIMITS.maxPixels});
 }
 const overlay=Buffer.from(width<320||height<100
  ?`<svg width="${width}" height="${height}" xmlns="http://www.w3.org/2000/svg"><rect x="4" y="${height-28}" width="${Math.min(width-8,100)}" height="24" rx="4" fill="#fff" opacity=".9"/><text x="8" y="${height-11}" font-family="sans-serif" font-size="${Math.min(18,(width-16)/3.5)}" fill="#46584b">MOCK</text></svg>`
  :`<svg width="${width}" height="${height}" xmlns="http://www.w3.org/2000/svg"><rect x="24" y="${height-65}" width="275" height="40" rx="6" fill="#fff" opacity=".85"/><text x="39" y="${height-37}" font-family="sans-serif" font-size="21" fill="#46584b">MOCK · LOCAL SAMPLE ${index+1}</text></svg>`);
 return image.composite([{input:overlay}]).png().toBuffer();
}
export class MockProvider implements ImageProvider {
 capabilities={...OPENAI_CAPABILITIES,provider:'mock',model:'mock-image-v1'};
 async submit(snapshot:InputSnapshot,source?:Buffer):Promise<ProviderResult>{ validateProviderInput(snapshot); const images:Buffer[]=[]; for(let i=0;i<snapshot.params.n;i++) images.push(await mockImage(i,source,snapshot.params.size)); return {images,providerRequestId:`mock-${crypto.randomUUID()}`,usage:{mock:true,images:images.length}}; }
}
/** No automatic retries, queries or cancellation are claimed for the synchronous Images API. */
export class OpenAIProvider implements ImageProvider {
 capabilities=OPENAI_CAPABILITIES;
 async submit(snapshot:InputSnapshot,source?:Buffer):Promise<ProviderResult>{
  assert(snapshot.provider==='openai','供应商快照与独立 API 适配器不匹配');validateProviderInput(snapshot); assert(snapshot.model==='gpt-image-1.5','此适配器仅核实了 gpt-image-1.5；切换模型须更新能力配置');
  const key=process.env.OPENAI_API_KEY; assert(key,'服务端尚未配置 API 密钥',403,'authorization');
  const headers:Record<string,string>={Authorization:`Bearer ${key}`};
  if(snapshot.providerOrganization) headers['OpenAI-Organization']=snapshot.providerOrganization;
  if(snapshot.providerProject) headers['OpenAI-Project']=snapshot.providerProject;
  let body:BodyInit;
  if(snapshot.nodeKind==='edit') { assert(source,'缺少编辑主图'); const form=new FormData(); form.set('model',snapshot.model);form.set('prompt',snapshot.prompt);form.set('size',snapshot.params.size);form.set('quality',snapshot.params.quality);form.set('n',String(snapshot.params.n));form.set('output_format','png');const png=await sharp(source,{limitInputPixels:40_000_000}).png().toBuffer();form.set('image',new Blob([new Uint8Array(png)],{type:'image/png'}),'source.png');body=form; }
  else {headers['Content-Type']='application/json';body=JSON.stringify({model:snapshot.model,prompt:snapshot.prompt,...snapshot.params,output_format:'png'});}
  let response:Response;
  try {response=await fetch(`https://api.openai.com/v1/images/${snapshot.nodeKind==='edit'?'edits':'generations'}`,{method:'POST',headers,body,signal:AbortSignal.timeout(180_000)});} catch {throw new AppError('网络中断或响应超时，是否受理未知。请先在供应商侧对账；本工具不会自动重发。',502,'transport_unknown');}
  const requestId=response.headers.get('x-request-id')??undefined;
  if(!response.ok) {let code='';try{const payload=await response.json();code=String(payload?.error?.code??payload?.error?.type??'');}catch{}const cls=/quota|billing|credit/.test(code)?'quota':/policy|moderation|safety/.test(code)?'policy':response.status===401||response.status===403?'authorization':response.status===429?'rate_limit':response.status>=500?'transport_unknown':'provider_error';throw new AppError(response.status>=500?'供应商响应异常，受理状态未知，请先对账。':`供应商拒绝请求（HTTP ${response.status}），请检查授权、额度或内容要求。`,502,cls,requestId);}
  let data:{data?:{b64_json?:string}[];usage?:Record<string,unknown>};
  try {data=await response.json();}catch{throw new AppError('供应商响应无法解析，受理状态未知。',502,'transport_unknown',requestId);}
  const images=(data.data??[]).flatMap(item=>item.b64_json?[Buffer.from(item.b64_json,'base64')]:[]);
  if(!images.length) throw new AppError('供应商未返回可入库图片，请先对账。',502,'transport_unknown',requestId);
  return {images,providerRequestId:requestId,usage:data.usage};
 }
}
