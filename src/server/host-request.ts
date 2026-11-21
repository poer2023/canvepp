import {AppError,assert} from './errors';

/** Host control requests contain identifiers only; enforce the cap while reading. */
export async function hostRequestBody(request:Request):Promise<Record<string,unknown>>{
 const max=4096;assert(Number(request.headers.get('content-length')??0)<=max,'请求过大',413);
 const reader=request.body?.getReader();const chunks:Uint8Array[]=[];let total=0;
 if(reader)while(true){const {done,value}=await reader.read();if(done)break;total+=value.byteLength;if(total>max){await reader.cancel();throw new AppError('请求过大',413);}chunks.push(value);}
 const bytes=new Uint8Array(total);let offset=0;for(const chunk of chunks){bytes.set(chunk,offset);offset+=chunk.byteLength;}
 let body:unknown;try{body=JSON.parse(new TextDecoder().decode(bytes));}catch{throw new AppError('JSON 格式无效');}
 assert(body&&typeof body==='object'&&!Array.isArray(body),'请求格式无效');
 const record=body as Record<string,unknown>;assert(typeof record.requestId==='string'&&record.requestId.length>0&&record.requestId.length<=160,'缺少 requestId');return record;
}
