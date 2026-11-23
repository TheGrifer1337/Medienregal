'use strict';

function coverSource(item) {
  if (item.customCover?.startsWith('data:image/jpeg;base64,')) return item.customCover;
  return httpsURL(item.customCover)||httpsURL(item.cover);
}

async function prepareCover(file) {
  if (!['image/jpeg','image/png','image/webp','image/gif','image/bmp','image/avif'].includes(file.type)||file.size>20*1024*1024) {
    throw new Error(tr('Wähle eine Bilddatei mit höchstens 20 MB.'));
  }
  const bitmap=await createImageBitmap(file);
  try {
    const canvas=document.createElement('canvas');
    const ratio=Math.min(1,500/bitmap.width,750/bitmap.height);
    canvas.width=Math.max(1,Math.round(bitmap.width*ratio));
    canvas.height=Math.max(1,Math.round(bitmap.height*ratio));
    const ctx=canvas.getContext('2d');
    ctx.fillStyle='#17131b';
    ctx.fillRect(0,0,canvas.width,canvas.height);
    ctx.drawImage(bitmap,0,0,canvas.width,canvas.height);
    for (const quality of [0.84,0.68,0.48,0.3]) {
      const image=canvas.toDataURL('image/jpeg',quality);
      if (image.length<=330000) return image;
    }
    throw new Error(tr('Das Bild konnte nicht ausreichend verkleinert werden.'));
  } finally { bitmap.close(); }
}

function customCoverEditor(item,refresh) {
  const section=text('details',null,'genre-picker cover-editor');
  section.append(text('summary',tr('Eigenes Cover')));
  const file=input('file','custom-cover-file');
  file.accept='image/jpeg,image/png,image/webp,image/gif,image/bmp,image/avif';
  const address=input('url','custom-cover-url',httpsURL(item.customCover));
  const status=text('p',null,'form-note');
  status.setAttribute('role','status');
  let local=item.customCover?.startsWith('data:image/jpeg;base64,')?item.customCover:'';
  let revision=0;
  let loading=false;
  const value=()=>address.value.trim()||local;
  const reset=button(tr('Standardcover wiederherstellen'),()=>{
    revision++;
    loading=false;
    file.value='';address.value='';local='';
    status.textContent=tr('Beim Speichern wird das Standardcover übernommen.');
    refresh('');
  },'quiet');
  address.addEventListener('input',()=>refresh(value()));
  file.addEventListener('change',async()=>{
    const selected=file.files[0];
    if(!selected) return;
    const generation=++revision;
    loading=true;
    status.textContent=tr('Bild wird vorbereitet …');
    try {
      const prepared=await prepareCover(selected);
      if(generation!==revision) return;
      local=prepared;address.value='';
      status.textContent=tr('Bild ausgewählt.');
      refresh(local);
    } catch(e) {
      if(generation===revision) status.textContent=e.message||tr('Die Bilddatei konnte nicht gelesen werden.');
    } finally {
      if(generation===revision) {loading=false;file.value='';}
    }
  });
  section.append(field(tr('Bilddatei auswählen'),file),field(tr('Oder HTTPS-Bildadresse'),address),status,reset);
  return {section,value,loading:()=>loading};
}

const coverCache=new Map();
const coverQueue=[];
let coverRequests=0;
const pendingCovers=new Map();
const visibleCovers=new IntersectionObserver(changes=>{
  for (const {target,isIntersecting} of changes) {
    if (!isIntersecting) continue;
    const search=pendingCovers.get(target);
    visibleCovers.unobserve(target);
    pendingCovers.delete(target);
    if (target.isConnected) search?.();
  }
});
new MutationObserver(()=>{
  for (const node of pendingCovers.keys()) {
    if (node.isConnected) continue;
    visibleCovers.unobserve(node);
    pendingCovers.delete(node);
  }
}).observe(document.body,{childList:true,subtree:true});

function runCoverQueue() {
  while (coverRequests<3 && coverQueue.length) {
    const {endpoint,resolve,saved}=coverQueue.shift();
    coverRequests++;
    api(endpoint).then(items=>{
      saved.until=Date.now()+(items.length?3600000:180000);
      resolve(items);
    }).catch(()=>{
      saved.until=Date.now()+30000;
      resolve([]);
    }).finally(()=>{
      coverRequests--;
      runCoverQueue();
    });
  }
}

function findArtwork(item) {
  const params=new URLSearchParams({kind:item.kind,title:item.title,subtitle:(item.subtitle||'').slice(0,260)});
  const id=item.catalogId||item.id;
  if (id?.startsWith(item.kind+':')) params.set('id',id);
  const isbn=(item.cover||'').match(/^https:\/\/covers\.openlibrary\.org\/b\/isbn\/([0-9X]+)-[SML]\.jpg/)?.[1];
  if (isbn) params.set('isbn',isbn);
  for (const alias of (item.aliases||[]).slice(0,4)) params.append('alias',alias.slice(0,200));
  while (String(params).length>4000) {
    if (params.has('alias')) params.delete('alias');
    else if (params.has('subtitle')) params.delete('subtitle');
    else return Promise.resolve([]);
  }
  const endpoint='cover?'+params;
  const cached=coverCache.get(endpoint);
  if (cached?.until>Date.now()) return cached.promise;
  if (coverCache.size>=500) coverCache.delete(coverCache.keys().next().value);
  const saved={until:Infinity};
  saved.promise=new Promise(resolve=>coverQueue.push({endpoint,resolve,saved}));
  coverCache.set(endpoint,saved);
  runCoverQueue();
  return saved.promise;
}

function cover(item,credit) {
  const letters=item.title.trim().split(/\s+/).slice(0,2).map(word=>Array.from(word)[0]).join('').toUpperCase();
  const node=text('span',letters||definitions[item.kind].icon,'cover');
  node.setAttribute('aria-hidden','true');
  credit?.replaceChildren();
  const tried=new Set();

  function showArtwork(candidate,failed) {
    const url=httpsURL(candidate.url)||((candidate.url||'').startsWith('data:image/jpeg;base64,')?candidate.url:'');
    if (!url||tried.has(url)) { failed();return; }
    tried.add(url);
    const image=document.createElement('img');
    image.alt='';image.loading='lazy';image.referrerPolicy='no-referrer';
    const remove=()=>{ image.remove(); if (node.isConnected) failed(); };
    image.addEventListener('load',()=>{
      if (image.naturalWidth<=1||image.naturalHeight<=1) {remove();return;}
      if (!candidate.source) return;
      node.title=tr('Cover von ')+candidate.source;
      if (credit && node.isConnected) {
        const label=tr('Cover von ')+candidate.source;
        credit.replaceChildren(httpsURL(candidate.page)?link(candidate.page,label):label);
      }
    },{once:true});
    image.addEventListener('error',remove,{once:true});
    node.append(image);
    image.src=url;
  }

  function search() {
    if (!item.title.trim()) return;
    pendingCovers.set(node,async()=>{
      const results=await findArtwork(item);
      if (!node.isConnected) return;
      const alternatives=results.filter(candidate=>httpsURL(candidate.url));
      const next=()=>{ if (alternatives.length) showArtwork(alternatives.shift(),next); };
      next();
    });
    visibleCovers.observe(node);
  }

  const standard=httpsURL(item.cover);
  const custom=coverSource({customCover:item.customCover});
  const showStandard=()=>standard?showArtwork({url:standard},search):search();
  if (custom) showArtwork({url:custom},showStandard);
  else showStandard();
  return node;
}
