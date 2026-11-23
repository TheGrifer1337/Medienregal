'use strict';

const movieInfoQueue=[];
const movieInfoCache=new Map();
let movieInfoActive=0;
const trailerCache=new Map();

function movieParams(item) {
  const params={id:catalogID(item),title:item.title,year:(item.subtitle||'').match(/^\s*((?:18|19|20)\d{2})\b/)?.[1]||''};
  return params;
}

function loadMovieRatings() {
  while (movieInfoActive<3 && movieInfoQueue.length) {
    const job=movieInfoQueue.shift();
    movieInfoActive++;
    api(job.endpoint).then(job.resolve,()=>{
      movieInfoCache.delete(job.endpoint);
      job.resolve({});
    }).finally(()=>{movieInfoActive--;loadMovieRatings();});
  }
}

const observedMovies=new Map();
const movieObserver=new IntersectionObserver(changes=>{
  for (const {target,isIntersecting} of changes) {
    if (!isIntersecting) continue;
    const find=observedMovies.get(target);
    movieObserver.unobserve(target);
    observedMovies.delete(target);
    if (target.isConnected) find?.();
  }
});
new MutationObserver(()=>{
  for (const node of observedMovies.keys()) {
    if (node.isConnected) continue;
    observedMovies.delete(node);
    movieObserver.unobserve(node);
  }
}).observe(document.body,{childList:true,subtree:true});

function lookupMovieRating(item) {
  const endpoint=route('imdb-rating',movieParams(item));
  let pending=movieInfoCache.get(endpoint);
  if (!pending) {
    if (movieInfoCache.size>=600) movieInfoCache.delete(movieInfoCache.keys().next().value);
    pending=new Promise(resolve=>movieInfoQueue.push({endpoint,resolve}));
    movieInfoCache.set(endpoint,pending);
    loadMovieRatings();
  }
  return pending;
}

function movieBadge(item, detail=false) {
  const node=text(detail?'p':'small','IMDb …','imdb-badge');
  const paint=rating=>{
    node.textContent=rating.score?'IMDb '+number.format(rating.score)+' / 10':'IMDb —';
    node.title=rating.score?'IMDb':tr('Keine IMDb-Bewertung verfügbar');
    if (detail && /^tt\d+$/.test(rating.id||'')) {
      node.replaceChildren(link('https://www.imdb.com/title/'+rating.id+'/',node.textContent));
    }
  };
  const id=catalogID(item).match(/^movie:imdb:(tt\d+)$/)?.[1];
  if (item.imdb) paint({score:item.imdb,id});
  else {
    observedMovies.set(node,async()=>{
      const rating=await lookupMovieRating(item);
      if(node.isConnected) paint(rating);
    });
    movieObserver.observe(node);
  }
  return node;
}

function parseTrailer(value) {
  if (/^[\w-]{11}$/.test(value)) return value;
  try {
    const url=new URL(value);
    if (url.protocol!=='https:' || url.username || url.password) return '';
    const [section,key]=url.pathname.split('/').filter(Boolean);
    let id='';
    if (url.hostname==='youtu.be') id=section;
    if (['www.youtube.com','youtube.com','m.youtube.com','www.youtube-nocookie.com'].includes(url.hostname)) {
      id=section==='watch'?url.searchParams.get('v'):['embed','shorts'].includes(section)?key:'';
    }
    return /^[\w-]{11}$/.test(id||'')?id:'';
  } catch { return ''; }
}

function movieTrailerPanel(item) {
  const panel=text('details',null,'genre-picker media-extra');
  panel.append(text('summary',tr('Trailer')));
  const own=input('text','trailer',item.trailer?'https://youtu.be/'+item.trailer:'',tr('YouTube-Link'));
  own.maxLength=350;
  const box=text('div',null,'movie-trailer-player');
  const message=text('p',null,'form-note');
  const play=button(tr('Trailer ansehen'),async()=>{
    let id=parseTrailer(own.value.trim());
    if (!id && own.value.trim()) { message.textContent=tr('Gib einen gültigen YouTube-Link ein.');return; }
    play.disabled=true;
    try {
      if (!id) {
        message.textContent=tr('Trailer wird gesucht …');
        const endpoint=route('movie-trailer',movieParams(item));
        let pending=trailerCache.get(endpoint);
        if (!pending) {
          pending=api(endpoint).catch(()=>{trailerCache.delete(endpoint);return {};});
          trailerCache.set(endpoint,pending);
        }
        id=(await pending).trailer;
      }
      if (!panel.isConnected || !modal.open) return;
      message.textContent=id?'':tr('Kein Trailer gefunden. Du kannst einen eigenen Link eintragen.');
      if (id) box.replaceChildren(trailerPlayer(id,item.title));
    } finally { play.disabled=false; }
  },'quiet');
  own.addEventListener('input',()=>{box.replaceChildren();message.textContent='';});
  panel.addEventListener('toggle',()=>{if(!panel.open)box.replaceChildren();});
  panel.append(field(tr('Eigener Trailer (optional)'),own),play,message,box);
  return {panel,value:()=>own.value.trim(),id:()=>parseTrailer(own.value.trim())};
}

function playbackAction(item) {
  if (!isScreen(item) && item.kind!=='game') return null;
  const control=button(item.kind==='game'?tr('Spiel starten'):tr('Ansehen'),()=>{
    const inner=show(item.title);
    const note=advisoryPanel(item);
    inner.append(note);
    if (isScreen(item)) {
      const loading=text('p',tr('Streamingangebote werden geladen …'),'form-note');
      inner.append(loading);
      if (item.kind==='movie' && !streamQuery(item) && (state.prefs?.hasWatchmode || state.prefs?.hasTmdb)) {
        lookupMovieRating(item).then(found=>{
          if (!inner.isConnected || !modal.open) return;
          const film=/^tt\d+$/.test(found.id||'')?{...item,catalogId:'movie:imdb:'+found.id}:item;
          loading.replaceWith(streamPanel(film));
        });
      } else loading.replaceWith(streamPanel(item));
      return;
    }
    const error=text('p',null,'error playback-error');
    const launch=button(tr('Spiel starten'),async()=>{
      launch.disabled=true;
      try { await api('launch-game','POST',{id:catalogID(item),title:item.title}); }
      catch(e) { error.textContent=e.message; }
      finally { launch.disabled=false; }
    },'accent');
    inner.append(launch,error);
  },'quiet');
  const group=text('div',null,'playback-action');
  group.append(control);
  return group;
}

function seriesAndLoanEditor(item) {
  const panel=text('details',null,'genre-picker media-extra');
  panel.append(text('summary',tr('Reihe und Verleih')));
  const name=input('text','series',item.series);
  const order=input('number','sequence',item.sequence||'');
  order.min='0';order.max='100000';order.step='any';
  name.maxLength=120;
  const names=text('datalist');
  names.id='known-series';
  for (const series of [...new Set(state.entries.map(entry=>entry.series).filter(Boolean))]) names.append(option(series,series));
  name.setAttribute('list',names.id);
  const to=input('text','borrower',item.loan?.to);
  to.maxLength=120;
  const since=input('date','loan-since',item.loan?.since);
  const due=input('date','loan-due',item.loan?.due);
  const clear=button(tr('Als zurückgegeben markieren'),()=>{to.value='';since.value='';due.value='';},'quiet');
  panel.append(field(tr('Reihe oder Universum'),name),names,field(tr('Position in der Reihe'),order));
  if (item.series) panel.append(button(tr('Reihenfolge anzeigen'),showSeriesDialog.bind(null,item.series,item.id),'quiet'));
  if (item.kind==='movie' && state.prefs?.hasTmdb && /^movie:(imdb|tmdb):/.test(catalogID(item))) {
    const catalog=text('div',null,'series-catalog');
    const find=button(tr('Filmreihe aus dem Katalog laden'),async()=>{
      find.disabled=true;
      catalog.replaceChildren(text('p',tr('Lädt …'),'form-note'));
      try {
        const data=await api(route('movie-series',{id:catalogID(item)}));
        if (!panel.isConnected) return;
        if (!data.items.length) {
          catalog.replaceChildren(text('p',tr('Keine Filmreihe im Katalog gefunden.'),'form-note'));
          return;
        }
        const list=text('ol',null,'media-overview');
        for (const part of data.items) {
          const row=text('li');
          row.append(button(part.title,()=>selectResult(part),'quiet'),text('small',part.subtitle));
          list.append(row);
        }
        catalog.replaceChildren(text('strong',data.name),text('p',tr('Nach Veröffentlichung sortiert'),'form-note'),list);
        const position=data.items.findIndex(part=>catalogID(part)===catalogID(item) || matchWords(part.title)===matchWords(item.title)&&part.subtitle===item.subtitle);
        if (position>=0) catalog.append(button(tr('Reihe und Position übernehmen'),()=>{
          name.value=data.name;order.value=position+1;
        },'quiet'));
      } catch(e) { catalog.replaceChildren(text('p',e.message,'error')); }
      finally { find.disabled=false; }
    },'quiet');
    panel.append(find,catalog);
  }
  panel.append(field(tr('Verliehen an'),to),field(tr('Verliehen seit'),since),field(tr('Rückgabe bis'),due));
  if (item.loan) panel.append(clear);
  return {
    panel,
    fields:()=>({
      series:name.value.trim(),sequence:name.value.trim()?Number(order.value)||0:0,
      loan:to.value.trim()?{to:to.value.trim(),since:since.value,due:due.value}:null,
    }),
  };
}

function showSeriesDialog(name,activeID='') {
  const inner=show(name||tr('Reihen und Universen'));
  const names=[...new Set(state.entries.map(item=>item.series).filter(Boolean))].sort(collator.compare);
  if (!name) {
    if (!names.length) inner.append(text('p',tr('Ordne einen gespeicherten Titel in seiner Detailansicht einer Reihe zu.'),'form-note'));
    for (const series of names) inner.append(button(series,()=>showSeriesDialog(series),'quiet'));
    return;
  }
  inner.append(button(tr('Alle Reihen'),()=>showSeriesDialog(),'quiet'));
  const list=text('ol',null,'media-overview');
  const parts=state.entries.filter(item=>matchWords(item.series||'')===matchWords(name))
    .sort((a,b)=>(a.sequence||Infinity)-(b.sequence||Infinity)||collator.compare(a.title,b.title));
  for (const item of parts) {
    const row=text('li');
    const control=button(item.title,()=>openEditor(item),'quiet');
    if (item.id===activeID) control.setAttribute('aria-current','true');
    row.append(text('span',item.sequence?number.format(item.sequence):'—'),control,text('small',tr(definitions[item.kind].title)));
    list.append(row);
  }
  inner.append(list);
}

function openLoans() {
  const inner=show(tr('Verliehene Medien'));
  const loans=state.entries.filter(item=>item.loan?.to)
    .sort((a,b)=>(a.loan.due||'9999').localeCompare(b.loan.due||'9999')||collator.compare(a.title,b.title));
  if (!loans.length) inner.append(text('p',tr('Du hast aktuell keine verliehenen Medien.'),'form-note'));
  const list=text('ul',null,'media-overview');
  for (const item of loans) {
    const date=value=>value?new Intl.DateTimeFormat(state.language,{dateStyle:'medium',timeZone:'UTC'}).format(new Date(value+'T00:00:00Z')):'';
    const row=text('li');
    row.append(button(item.title,()=>openEditor(item),'quiet'),text('small',[item.loan.to,date(item.loan.since),date(item.loan.due)].filter(Boolean).join(' · ')));
    list.append(row);
  }
  inner.append(list);
}

function yearGoals(year,completed) {
  const panel=text('section',null,'stat-tile');
  panel.append(text('h2',tr('Jahresziele')));
  const form=text('form',null,'goal-fields');
  const error=text('p',null,'error');
  const save=text('button',tr('Ziele speichern'),'accent');
  save.type='submit';
  const fields=Object.keys(definitions).map(kind=>{
    const target=state.goals?.[year]?.[kind]||0;
    const count=completed.filter(item=>item.kind===kind).length;
    const control=input('number',kind,target||'');
    control.min='0';control.max='10000';control.step='1';
    const label=field(tr(definitions[kind].title),control);
    if (target) {
      const progress=text('progress');
      progress.max=target;
      progress.value=Math.min(count,target);
      label.append(progress,text('small',count+' / '+target));
    }
    form.append(label);
    return control;
  });
  form.append(save,error);
  panel.append(text('p',String(year),'form-note'),form);
  form.addEventListener('submit',async event=>{
    event.preventDefault();
    save.disabled=true;
    try {
      const targets=Object.fromEntries(fields.map(({name,value})=>[name,Number(value)||0]));
      state.goals=await api('goals','PUT',{year,targets});
      if (state.kind==='stats' && panel.isConnected) collectionStatistics();
    } catch(e) { error.textContent=e.message;save.disabled=false; }
  });
  return panel;
}

function barcodeControls(inner,form,query,kind) {
  const box=text('details',null,'genre-picker media-extra');
  box.append(text('summary',tr('ISBN oder Barcode scannen')));
  const code=input('text','barcode','',tr('ISBN, EAN oder UPC'));
  code.inputMode='numeric';
  code.maxLength=25;
  const preview=text('video',null,'barcode-preview');
  preview.muted=true;preview.playsInline=true;preview.hidden=true;
  const notice=text('p',null,'form-note');
  const file=input('file','barcode-image');
  file.accept='image/*';
  let stream=null;
  let generation=0;
  const stop=()=>{
    generation++;
    stream?.getTracks().forEach(track=>track.stop());
    stream=null;preview.srcObject=null;preview.hidden=true;
    camera.disabled=false;
  };
  const use=async()=>{
    const value=code.value.trim().replace(/[\s-]/g,'').toUpperCase();
    if (kind==='book' && /^(?:97[89]\d{10}|\d{9}[0-9X])$/.test(value)) {
      query.value=value;form.requestSubmit();return;
    }
    try {
      notice.textContent=tr('Barcode wird gesucht …');
      const product=await api(route('barcode',{code:value}));
      if (!inner.isConnected || !modal.open) return;
      // Search results remain editable and always require the user to select a match.
      query.value=product.title;
      notice.textContent=tr('Produkt aus dem Barcode-Katalog übernommen. Passe den Suchbegriff bei Bedarf an.');
      form.requestSubmit();
    } catch(e) { notice.textContent=e.message; }
  };
  const frame=async image=>{
    const width=image.videoWidth||image.width,height=image.videoHeight||image.height;
    const scale=Math.min(1,1000/Math.max(width,height));
    const canvas=document.createElement('canvas');
    canvas.width=Math.round(width*scale);canvas.height=Math.round(height*scale);
    canvas.getContext('2d').drawImage(image,0,0,canvas.width,canvas.height);
    const blob=await new Promise(resolve=>canvas.toBlob(resolve,'image/jpeg',0.85));
    const result=await fetch('api/barcode',{method:'POST',body:blob});
    if (result.status===204) return '';
    const data=await result.json();
    if (!result.ok) throw new Error(tr(data.error));
    return data.code;
  };
  const camera=button(tr('Kamera einschalten'),async()=>{
    camera.disabled=true;
    const turn=++generation;
    try {
      if (!navigator.mediaDevices?.getUserMedia) throw new Error('camera unavailable');
      const live=await navigator.mediaDevices.getUserMedia({video:{width:{ideal:1280},facingMode:{ideal:'environment'}},audio:false});
      if (turn!==generation || !inner.isConnected || !modal.open) {
        live.getTracks().forEach(track=>track.stop());return;
      }
      stream=live;preview.srcObject=stream;preview.hidden=false;await preview.play();
      notice.textContent=tr('Halte den Barcode vor die Kamera.');
      while (turn===generation && inner.isConnected && modal.open && box.open) {
        const result=await frame(preview);
        if (turn!==generation) return;
        if (result) {stop();code.value=result;await use();return;}
        await new Promise(resolve=>setTimeout(resolve,250));
      }
      if (turn===generation) stop();
    } catch {
      if (turn===generation) {stop();notice.textContent=tr('Die Kamera ist nicht verfügbar. Nutze ein Foto oder gib den Code ein.');}
    }
  },'quiet');
  file.addEventListener('change',async()=>{
    if (!file.files[0]) return;
    stop();
    const turn=generation;
    try {
      const bitmap=await createImageBitmap(file.files[0]);
      let result;
      try { result=await frame(bitmap); } finally { bitmap.close(); }
      if (turn!==generation || !inner.isConnected || !modal.open) return;
      if (result) {code.value=result;await use();}
      else notice.textContent=tr('Auf dem Foto wurde kein Barcode erkannt.');
    } catch { notice.textContent=tr('Das Bild konnte nicht gelesen werden.'); }
    finally {file.value='';}
  });
  box.addEventListener('toggle',()=>{if(!box.open)stop();});
  const close=()=>{stop();observer.disconnect();};
  const observer=new MutationObserver(()=>{if(!inner.isConnected)close();});
  observer.observe(modalBody,{childList:true});
  modal.addEventListener('close',close,{once:true});
  box.append(field(tr('Code eingeben'),code),button(tr('Code suchen'),use,'quiet'),camera,
    button(tr('Kamera ausschalten'),stop,'quiet'),preview,field(tr('Foto auswählen'),file),notice);
  return box;
}
