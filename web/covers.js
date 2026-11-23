'use strict';

function coverSource(item) {
  if (item.customCover?.startsWith('data:image/jpeg;base64,')) return item.customCover;
  return httpsURL(item.customCover)||httpsURL(item.cover);
}

async function prepareCover(file) {
  if (!['image/jpeg','image/png','image/webp','image/gif','image/bmp','image/avif'].includes(file.type)||file.size>20*1024*1024) {
    throw new Error('Wähle eine Bilddatei mit höchstens 20 MB.');
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
    throw new Error('Das Bild konnte nicht ausreichend verkleinert werden.');
  } finally { bitmap.close(); }
}

function customCoverEditor(item,refresh) {
  const section=text('details',null,'genre-picker cover-editor');
  section.append(text('summary','Eigenes Cover'));
  const file=input('file','custom-cover-file');
  file.accept='image/jpeg,image/png,image/webp,image/gif,image/bmp,image/avif';
  const address=input('url','custom-cover-url',httpsURL(item.customCover));
  const status=text('p',null,'form-note');
  status.setAttribute('role','status');
  let local=item.customCover?.startsWith('data:image/jpeg;base64,')?item.customCover:'';
  let revision=0;
  let loading=false;
  const value=()=>address.value.trim()||local;
  const reset=button('Standardcover wiederherstellen',()=>{
    revision++;
    loading=false;
    file.value='';address.value='';local='';
    status.textContent='Beim Speichern wird das Standardcover übernommen.';
    refresh('');
  },'quiet');
  address.addEventListener('input',()=>refresh(value()));
  file.addEventListener('change',async()=>{
    const selected=file.files[0];
    if(!selected) return;
    const generation=++revision;
    loading=true;
    status.textContent='Bild wird vorbereitet …';
    try {
      const prepared=await prepareCover(selected);
      if(generation!==revision) return;
      local=prepared;address.value='';
      status.textContent='Bild ausgewählt.';
      refresh(local);
    } catch(e) {
      if(generation===revision) status.textContent=e.message||'Die Bilddatei konnte nicht gelesen werden.';
    } finally {
      if(generation===revision) {loading=false;file.value='';}
    }
  });
  section.append(field('Bilddatei auswählen',file),field('Oder HTTPS-Bildadresse',address),status,reset);
  return {section,value,loading:()=>loading};
}
