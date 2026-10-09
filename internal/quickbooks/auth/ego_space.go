package auth

// Named lookup must resolve an existing numeric space before taskSpace runs:
// passing a missing name directly would silently create a new browser context.
const ExistingEgoTaskJS = `(async (key,open,spaces)=>{
 const refuse=code=>{const error=new Error(code);error.qbCode=code;throw error};
 let id=Number(key);
 if(!/^[0-9]+$/.test(key)){
  if(typeof spaces!=='function')refuse('pinned-page-unavailable');
  const matches=(await spaces()).filter(space=>space.name===key);
  if(matches.length!==1)refuse('pinned-page-unavailable');
  if(matches[0].ownership!=='agent')refuse('user-control');
  id=matches[0].id;
 }
 if(!Number.isSafeInteger(id)||id<1)refuse('pinned-page-unavailable');
 return await open(id);
})`
