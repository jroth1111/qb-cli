package auth

// In-page fetch needs a byte representation: text conversion changes arbitrary
// uploads, and SDK inline response text may be capped. Callers verify the owning
// page and destination before and after evaluating this expression.
const BrowserByteFetchJS = `(async request=>{
 const options={method:request.method,headers:request.headers,credentials:'include',redirect:'error',cache:'no-store',signal:AbortSignal.timeout(request.timeoutMs)};
 if(request.method!=='GET'&&request.method!=='HEAD'&&request.bodyBase64)options.body=Uint8Array.from(atob(request.bodyBase64),character=>character.charCodeAt(0));
 const response=await fetch(request.endpoint,options),bytes=new Uint8Array(await response.arrayBuffer());
 let encoded='';for(let index=0;index<bytes.length;index+=32768)encoded+=String.fromCharCode(...bytes.subarray(index,index+32768));
 return {status:response.status,headers:Object.fromEntries(response.headers),bodyBase64:btoa(encoded)};
})`
