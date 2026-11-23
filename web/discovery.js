'use strict';

let steamSnapshot=null;
const advisoryCache=new Map();
const recommendationCache=new Map();

function steamPlaytime(item) {
  const label=text('small',null,'steam-activity muted');
  const id=catalogID(item).match(/^game:steam:(\d+)$/)?.[1];
  if (!id) return label;
  if (!steamSnapshot || steamSnapshot.until<Date.now()) {
    steamSnapshot={until:Date.now()+600000,request:api('steam-info').catch(()=>({}))};
  }
  steamSnapshot.request.then(games=>{
    const game=games[id];
    if (!game||!label.isConnected) return;
    const hours=number.format(game.playtime_forever/60)+tr(' Std. auf Steam');
    const played=game.rtime_last_played?new Intl.DateTimeFormat(state.language,{dateStyle:'medium'}).format(game.rtime_last_played*1000):'';
    label.textContent=hours+(played?' · '+tr('Zuletzt gespielt: ')+played:'');
  });
  return label;
}

function advisoryPanel(item) {
  const panel=text('section',null,'advisory-panel');
  panel.append(text('strong',tr('Altersfreigabe und Inhaltswarnungen')));
  const status=text('p',tr('Lädt …'),'form-note');
  panel.append(status);
  const itemID=catalogID(item);
  const endpoint=route('advisories',{id:itemID});
  const key=[state.prefs?.region,state.language,endpoint].join(':');
  if (!advisoryCache.has(key)) {
    const request=async()=>{
      if (item.kind==='movie' && !/^movie:(?:tmdb|imdb):/.test(itemID)) {
        const found=await lookupMovieRating(item);
        if (/^tt\d+$/.test(found.id||'')) return api(route('advisories',{id:'movie:imdb:'+found.id}));
      }
      return api(endpoint);
    };
    advisoryCache.set(key,request().catch(()=>{advisoryCache.delete(key);return {};}));
  }
  advisoryCache.get(key).then(info=>{
    if (!panel.isConnected) return;
    const rating=info.ratings?.join(' / ');
    status.textContent=rating?regionName(state.prefs?.region||'DE',state.language)+' · '+rating+' ('+info.source+')':tr('Keine Altersfreigabe für dieses Land gefunden.');
    if (info.warnings?.length) {
      const list=text('ul');
      for (const warning of info.warnings) list.append(text('li',warning));
      panel.append(list);
    }
    if (info.note) panel.append(text('p',info.note,'form-note'));
    if (httpsURL(info.reference)) panel.append(link(info.reference,info.source==='Steam'?tr('Inhaltshinweise auf Steam'):tr('IMDb-Inhaltswarnungen öffnen')));
  });
  return panel;
}

function filterRatingGaps(items,cards) {
  const status=text('p',tr('IMDb-Bewertungen werden verglichen …'),'form-note filter-status');
  let available=0;
  const pending=items.map(async (item,index)=>{
    const card=cards[index];
    const personal=average(item);
    card.classList.add('gap-excluded');
    if (personal===null) return;
    const rating=item.imdb||((await lookupMovieRating(item)).score||0);
    if (rating && Math.abs(personal*2-rating)>=2) {
      card.classList.remove('gap-excluded');
      available++;
    }
  });
  Promise.all(pending).then(()=>{
    if (!status.isConnected) return;
    if (available) status.remove();
    else status.textContent=tr('Keine Filme mit beiden Bewertungen und mindestens 2 Punkten Unterschied.');
  });
  queueMicrotask(()=>{if(cards[0]?.isConnected)shelf.prepend(status);});
}

function escapeExport(value) {
  return String(value??'').replaceAll('&','&amp;').replaceAll('<','&lt;').replaceAll('>','&gt;').replaceAll('"','&quot;').replaceAll("'",'&#39;');
}

function listExportDocument(name,items) {
  const safe=escapeExport;
  const cards=items.map(item=>{
    const source=coverSource(item);
    const cover=source?'<img src="'+safe(source)+'" alt="" loading="lazy" referrerpolicy="no-referrer">':'';
    const score=average(item);
    return '<article>'+cover+'<div><h2>'+safe(item.title)+'</h2><p>'+safe([tr(definitions[item.kind].title),item.subtitle].filter(Boolean).join(' · '))+'</p><p>'+safe(score===null?tr('Ohne Bewertung'):'★ '+number.format(score)+' / 5')+'</p></div></article>';
  }).join('');
  return `<!doctype html><html lang="${safe(state.language)}"><meta charset="UTF-8"><meta name="viewport" content="width=device-width, initial-scale=1"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; img-src https: data:; style-src 'unsafe-inline'; base-uri 'none'"><title>${safe(name)}</title><style>body{font:1rem/1.55 system-ui,sans-serif;background:#16131c;color:#fbf4ed;max-width:1040px;margin:2rem auto;padding:0 1rem}main{display:grid;gap:1rem;grid-template-columns:repeat(auto-fit,minmax(min(100%,240px),1fr))}article{background:#241e2c;padding:1rem;border:1px solid #504157;border-radius:12px}h2{font-size:1.15rem;margin:.35rem 0}p{margin:.25rem 0}img{width:100%;aspect-ratio:2/2.7;object-fit:contain;background:#1a1520;border-radius:8px}</style><h1>${safe(name)}</h1><main>${cards}</main></html>`;
}

function showListExport() {
  const inner=show(tr('Liste als HTML exportieren'));
  const picker=document.createElement('select');
  picker.append(option('',tr('Aktueller Medienbereich')),...state.lists.map(name=>option(name,name)));
  picker.value=state.list||'';
  const update=()=>{
    const list=picker.value;
    const items=(list?state.entries.filter(item=>item.lists?.includes(list)):state.entries.filter(item=>item.kind===state.kind)).sort(compare);
    preview.textContent=items.length+' '+tr('gespeicherte Titel');
    save.disabled=!items.length;
  };
  const preview=text('p',null,'form-note');
  const save=button(tr('HTML-Datei herunterladen'),async()=>{
    const list=picker.value;
    const name=list||current().title;
    const items=(list?state.entries.filter(item=>item.lists?.includes(list)):state.entries.filter(item=>item.kind===state.kind)).sort(compare);
    const file=new File([listExportDocument(name,items)],(name.replace(/[^\p{L}\p{N}_-]+/gu,'-')||'Mediathek')+'.html',{type:'text/html'});
    if (mobileMode() && navigator.canShare?.({files:[file]})) {
      try { await navigator.share({files:[file],title:name});return; }
      catch(e) { if (e.name==='AbortError') return; }
    }
    const url=URL.createObjectURL(file);
    const anchor=document.createElement('a');
    anchor.href=url;anchor.download=file.name;inner.append(anchor);anchor.click();anchor.remove();
    setTimeout(()=>URL.revokeObjectURL(url),30000);
  },'accent');
  picker.addEventListener('change',update);
  inner.append(field(tr('Liste auswählen'),picker),preview,text('p',tr('Der Export enthält Titel, Cover und Bewertungen. Persönliche Notizen und Leihdaten bleiben in der Mediathek.'),'form-note'),save);
  update();
}

function paintRecommendations(saved) {
  const panel=$('recommendations');
  const heading=text('h2',tr('Für dich entdeckt'));
  heading.id='recommendations-heading';
  panel.replaceChildren(heading);
  const seeds=saved.filter(item=>item.favorite||(average(item)??0)>=4)
    .sort((a,b)=>((b.favorite?1:0)-(a.favorite?1:0))||((average(b)??0)-(average(a)??0))).slice(0,8);
  if (!seeds.length) {
    panel.append(text('p',tr('Markiere Favoriten oder bewerte Titel, um hier Empfehlungen zu sehen.'),'form-note'));
    return;
  }
  const kind=state.kind;
  const genres=new Map();
  for (const item of seeds) for (const genre of item.genres||[]) genres.set(genre,(genres.get(genre)||0)+(item.favorite?2:1));
  const chartGenres=state.prefs?.chartGenres?.[kind]||[];
  const preferred=[...genres].sort((a,b)=>b[1]-a[1]).map(([name])=>chartGenres.find(genre=>genreKey(genre)===genreKey(name))).find(Boolean);
  const author=kind==='book'?seeds[0].subtitle?.split(' · ')[0]:'';
  if (!preferred&&!author) {
    panel.append(text('p',tr('Ergänze Genres oder Buchautoren bei deinen Favoriten, um passende Titel zu finden.'),'form-note'));
    return;
  }
  const loading=text('p',tr('Lädt …'),'form-note');
  panel.append(loading);
  const key=[kind,preferred,author,state.prefs?.region,state.language].join(':');
  let cached=recommendationCache.get(key);
  if (!cached||cached.until<Date.now()) {
    const request=async()=>{
      if (author) {
        try {
          const list=await api(route('search',{kind,author}));
          const same=list.filter(item=>matchWords(item.subtitle||'').includes(matchWords(author)));
          if (same.some(item=>!saved.some(old=>catalogID(old)===catalogID(item)||matchWords(old.title)===matchWords(item.title)))) {
            return {items:same,reason:author};
          }
        } catch { /* Use the genre source when the author catalog is unavailable. */ }
      }
      if (!preferred) return {items:[]};
      const list=await api(route('rankings',{kind,genre:preferred}));
      return {items:list.items||[],reason:tr(preferred)};
    };
    cached={until:Date.now()+600000,request:request()};
    recommendationCache.set(key,cached);
  }
  cached.request.then(result=>{
    if (!heading.isConnected || state.kind!==kind) return;
    const known=new Set(saved.map(item=>matchWords(item.title)));
    const codes=new Set(saved.map(catalogID));
    const selected=[];
    for (const item of result.items) {
      const name=matchWords(item.title);
      if (known.has(name)||codes.has(catalogID(item))) continue;
      known.add(name);selected.push(item);
      if (selected.length===4) break;
    }
    if (!selected.length) {loading.textContent=tr('Gerade keine passenden neuen Titel gefunden.');return;}
    loading.textContent=tr('Aus deinen Favoriten und Bewertungen: ')+result.reason;
    for (const item of selected) {
      const control=button('',()=>selectResult(item),'recommendation-item quiet');
      control.append(cover(item),text('span',item.title));
      panel.append(control);
    }
  }).catch(()=>{recommendationCache.delete(key);if(loading.isConnected)loading.textContent=tr('Empfehlungen sind gerade nicht verfügbar.');});
}
