'use strict';

const charts = new Map();
const ticker = {kind:'',animation:null,generation:0,hovered:false,paused:matchMedia('(prefers-reduced-motion: reduce)').matches};
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
  if (ticker.kind===state.kind) { syncTicker();return; }
  ticker.kind=state.kind;
  const generation=++ticker.generation;
  ticker.animation?.cancel();
  ticker.animation=null;
  const kind=state.kind;
  const year=new Date().getFullYear();
  $('chart-title').textContent='Top 50 · '+year;
  $('chart-source').textContent='Lädt …';
  const track=$('chart-track');
  track.replaceChildren();
  $('chart-frame').scrollLeft=0;
  let pending=charts.get(kind);
  if (!pending) {
    pending=api(route('rankings',{kind}));
    charts.set(kind,pending);
  }
  pending.then(data=>{
    if (state.kind!==kind || generation!==ticker.generation) return;
    $('chart-title').textContent='Top '+data.items.length+' · '+data.year;
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
    charts.delete(kind);
    if (state.kind!==kind || generation!==ticker.generation) return;
    $('chart-source').textContent='Jahresliste gerade nicht verfügbar';
    const retry=button('Erneut laden',()=>{ticker.kind='';annualChart();},'quiet');
    track.append(retry);
    if (kind!=='book'&&!state.prefs?.hasTmdb) track.append(text('span','TMDB ist eine weitere Quelle in den Einstellungen.','form-note'));
  });
}

function initChart() {
  const pause=$('chart-pause');
  const syncPauseButton=()=>{
    pause.textContent=ticker.paused?'Abspielen':'Pausieren';
    pause.setAttribute('aria-pressed',String(ticker.paused));
  };
  syncPauseButton();
  pause.addEventListener('click',()=>{
    ticker.paused=!ticker.paused;syncPauseButton();
    if (!ticker.paused) {
      $('chart-frame').scrollLeft=0;
      if (!ticker.animation && $('chart-track').children.length) { ticker.kind='';annualChart(); }
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
  const titleCard=text('div',null,'ranking-detail');
  const info=text('div');
  const description=text('p',initial.description||'Kurzbeschreibung wird geladen …','ranking-description');
  const genres=text('p',genreLabels(initial.kind,initial.genres||[]).join(' · '),'form-note');
  info.append(text('p',initial.subtitle,'muted'),genres,description);
  titleCard.append(cover(initial),info);
  inner.append(titleCard);
  const player=text('div',null,'ranking-player');
  const film=initial.kind!=='book';
  if (film) player.append(text('p','Trailer wird gesucht …','form-note'));
  inner.append(player);
  const actions=text('div',null,'dialog-actions ranking-actions');
  const error=text('p',null,'error form-error');
  error.setAttribute('role','alert');
  const existing=state.entries.find(item=>catalogID(item)===initial.id);
  let item=initial;
  const states=film?[
    ['seen','Schon gesehen'],['want','Möchte ich sehen'],['started','Gerade dabei']
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
