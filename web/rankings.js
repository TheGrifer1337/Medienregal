'use strict';

const charts = new Map();
const ticker = {kind:'',genre:'',key:'',animation:null,generation:0,hovered:false,paused:matchMedia('(prefers-reduced-motion: reduce)').matches};
const catalogID = item => item.catalogId||item.id;

function syncTicker() {
  if (!ticker.animation) return;
  if (ticker.paused||ticker.hovered||modal.open||document.hidden||$('annual-chart').hidden) ticker.animation.pause();
  else ticker.animation.play();
}

function annualChart() {
  const panel=$('annual-chart');
  panel.hidden=state.kind==='stats';
  if (panel.hidden) { syncTicker();return; }
  const kind=state.kind;
  const genreControl=$('chart-genre');
  if (ticker.kind!==kind) {
    ticker.kind=kind;
    ticker.genre='';
    const genres=[...(state.prefs?.chartGenres?.[kind]||[])].sort(collator.compare);
    genreControl.replaceChildren(option('','Alle Genres'),...genres.map(genre=>option(genre,genre)));
  }
  const year=new Date().getFullYear();
  const key=JSON.stringify([kind,ticker.genre,year]);
  if (ticker.key===key) { syncTicker();return; }
  ticker.key=key;
  const generation=++ticker.generation;
  ticker.animation?.cancel();
  ticker.animation=null;
  const genre=ticker.genre;
  const chartTitle=count=>'Top '+count+' · '+(genre?genre+' · ':'')+year;
  $('chart-title').textContent=chartTitle(50);
  $('chart-source').textContent='Lädt …';
  const track=$('chart-track');
  track.replaceChildren();
  $('chart-frame').scrollLeft=0;
  let cached=charts.get(key);
  if (!cached || cached.expires<=Date.now()) {
    cached={request:api(route('rankings',{kind,genre})),expires:Date.now()+40*60*1000};
    charts.set(key,cached);
  }
  cached.request.then(data=>{
    if (state.kind!==kind || generation!==ticker.generation) return;
    $('chart-title').textContent=data.items.length?chartTitle(data.items.length):'Jahrescharts · '+year;
    if (!data.items.length) { $('chart-source').textContent='Keine Treffer für '+(genre||'dieses Jahr');return; }
    $('chart-source').textContent=data.source;
    const createStrip=clone=>{
      const strip=text('div',null,'chart-strip');
      if (clone) strip.setAttribute('aria-hidden','true');
      for (const [i,item] of data.items.entries()) {
        const control=button('',()=>openRankedTitle(item),'chart-item');
        control.setAttribute('aria-label','Platz '+(i+1)+': '+item.title);
        if (clone) control.tabIndex=-1;
        const content=text('span',null,'chart-caption');
        content.append(text('span','№ '+(i+1),'chart-rank'),text('strong',item.title));
        control.append(cover(item),content);strip.append(control);
      }
      return strip;
    };
    const strip=createStrip(false);
    track.append(strip,createStrip(true));
    if (strip.scrollWidth>$('chart-frame').clientWidth) {
      const distance=strip.getBoundingClientRect().width;
      ticker.animation=track.animate([{transform:'translateX(0)'},{transform:'translateX(-'+distance+'px)'}],
        {duration:distance/46*1000,iterations:Infinity});
      syncTicker();
    }
  }).catch(()=>{
    if(charts.get(key)===cached) charts.delete(key);
    if (state.kind!==kind || generation!==ticker.generation) return;
    $('chart-source').textContent='Jahresliste gerade nicht verfügbar';
    const retry=button('Erneut laden',()=>{ticker.key='';annualChart();},'quiet');
    track.append(retry);
    if ((kind==='movie'||kind==='series')&&!state.prefs?.hasTmdb) track.append(text('span','TMDB ist eine weitere Quelle in den Einstellungen.','form-note'));
  });
}

function initChart() {
  const pause=$('chart-pause');
  $('chart-genre').addEventListener('change',event=>{ ticker.genre=event.target.value;annualChart(); });
  setInterval(()=>{
    if (modal.open||document.hidden||ticker.hovered||$('annual-chart').hidden) return;
    if (charts.get(ticker.key)?.expires<=Date.now()) { ticker.key='';annualChart(); }
  },60000);
  const syncPauseButton=()=>{
    pause.textContent=ticker.paused?'Abspielen':'Pausieren';
    pause.setAttribute('aria-pressed',String(ticker.paused));
  };
  syncPauseButton();
  pause.addEventListener('click',()=>{
    ticker.paused=!ticker.paused;syncPauseButton();
    if (!ticker.paused) {
      $('chart-frame').scrollLeft=0;
      if (!ticker.animation && $('chart-track').children.length) { ticker.key='';annualChart(); }
    }
    syncTicker();
  });
  const frame=$('chart-frame');
  frame.addEventListener('pointerenter',()=>{ticker.hovered=true;syncTicker();});
  frame.addEventListener('pointerleave',()=>{ticker.hovered=false;syncTicker();});
  modal.addEventListener('close',()=>{
    ticker.hovered=false;
    modalBody.querySelectorAll('iframe').forEach(frame=>frame.remove());
    syncTicker();
  });
  document.addEventListener('visibilitychange',syncTicker);
  if (window.chrome?.webview) {
    document.addEventListener('fullscreenchange',()=>{
      api('fullscreen','POST',{enabled:Boolean(document.fullscreenElement)}).catch(()=>{});
    });
  }
  $('chart-track').addEventListener('keydown',event=>{
    if (event.key==='Tab'&&!ticker.paused) {
      ticker.paused=true;syncPauseButton();ticker.animation?.cancel();ticker.animation=null;
    }
  });
}

function trailerPlayer(id,title) {
  const container=text('div',null,'trailer-stage');
  const video=document.createElement('iframe');
  video.title='Trailer für '+title;
  video.referrerPolicy='origin';
  video.allow='autoplay; encrypted-media; fullscreen; picture-in-picture';
  video.allowFullscreen=true;
  video.src='https://www.youtube-nocookie.com/embed/'+id+'?autoplay=1&mute=0&playsinline=1&fs=1';
  const actions=text('div',null,'trailer-actions');
  actions.append(button('Vollbild',async()=>{
    try {
      if (document.fullscreenElement) await document.exitFullscreen();
      else await container.requestFullscreen();
    } catch { /* The player also exposes its own fullscreen control. */ }
  },'quiet'),link('https://www.youtube.com/watch?v='+id,'Auf YouTube öffnen'));
  container.append(video,actions);
  return container;
}

async function openRankedTitle(initial) {
  const inner=show(initial.title);
  const existing=state.entries.find(item=>catalogID(item)===initial.id);
  const titleCard=text('div',null,'ranking-detail');
  const info=text('div');
  const description=text('p',initial.description||'Kurzbeschreibung wird geladen …','ranking-description');
  const genres=text('p',genreLabels(initial.kind,initial.genres||[]).join(' · '),'form-note');
  info.append(text('p',initial.subtitle,'muted'),genres,description);
  const coverCredit=text('p',null,'form-note');
  info.append(coverCredit);
  const art=text('div');
  art.append(cover({...initial,cover:existing?.cover||initial.cover,customCover:existing?.customCover},coverCredit));
  titleCard.append(art,info);
  inner.append(titleCard);
  const player=text('div',null,'ranking-player');
  const film=isScreen(initial);
  if (film) player.append(text('p','Trailer wird gesucht …','form-note'));
  inner.append(player);
  const actions=text('div',null,'dialog-actions ranking-actions');
  const error=text('p',null,'error form-error');
  error.setAttribute('role','alert');
  let item=initial;
  const states=film?[
    ['seen','Schon gesehen'],['want','Möchte ich sehen'],['started','Gerade dabei']
  ]:initial.kind==='game'?[
    ['finished','Schon durchgespielt'],['want','Möchte ich spielen'],['started','Spiele ich gerade']
  ]:[
    ['finished','Schon gelesen'],['want','Möchte ich lesen'],['started','Lese ich gerade']
  ];
  for (const [status,label] of states) {
    const action=button(label,async()=>{
      const base=existing||item;
      const updated={...base,status,ratings:base.ratings||[0,0,0]};
      if (base.kind==='book'&&!base.genres?.length) { openEditor(updated);return; }
      actions.querySelectorAll('button').forEach(control=>control.disabled=true);
      try {
        await api('entries','PUT',updated);
        await refreshCollection();
        if (inner.isConnected) modal.close();
      } catch(e) {
        error.textContent=e.message;actions.querySelectorAll('button').forEach(control=>control.disabled=false);
      }
    },status==='want'?'accent':'quiet');
    if (existing?.status===status) action.setAttribute('aria-pressed','true');
    actions.append(action);
  }
  inner.append(error,actions);
  try {
    const found=await api(route('details',{id:initial.id,trailer:film?'1':'0',genres:'1'}));
    if (!modal.open||!inner.isConnected) return;
    item={...initial, description:found.description||initial.description, cover:found.cover||initial.cover,
      genres:found.genres?.length?found.genres:initial.genres,total:found.total||initial.total, aliases:found.aliases||initial.aliases};
    if (!existing?.cover && item.cover!==initial.cover) {
      art.replaceChildren(cover({...item,customCover:existing?.customCover},coverCredit));
    }
    description.textContent=item.description||'Keine Kurzbeschreibung verfügbar.';
    genres.textContent=genreLabels(item.kind,item.genres||[]).join(' · ');
    player.replaceChildren();
    if (film) {
      if (/^[A-Za-z0-9_-]{11}$/.test(found.trailer||'')) player.append(trailerPlayer(found.trailer,item.title));
      else player.append(text('p','Kein abspielbarer Trailer im Katalog gefunden.','form-note'));
    }
  } catch {
    if (!inner.isConnected) return;
    description.textContent=item.description||'Keine Kurzbeschreibung verfügbar.';
    if (film) player.replaceChildren(text('p','Trailer gerade nicht verfügbar.','form-note'));
  }
}
