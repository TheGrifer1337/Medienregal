'use strict';

const definitions = {
  movie: { title:'Filme', singular:'Film', icon:'F', states:{ seen:'Angesehen', want:'Möchte ich sehen' } },
  series: { title:'Serien', singular:'Serie', icon:'S', unit:'Folgen', states:{ seen:'Gesehen', started:'Angefangen', want:'Möchte ich sehen' } },
  book: { title:'Bücher', singular:'Buch', icon:'B', unit:'Seiten', states:{ read:'Gelesen', started:'Angefangen', finished:'Beendet', want:'Möchte ich lesen' } }
};
const ratings = { screen:['Qualität','Storyline','Allgemein'], book:['Storyline','Charakterentwicklung','Schreibstil'] };
const state = { kind:'movie', entries:[], status:'', genre:'', sort:'title', prefs:null, streamCache:new Map(), streamJobs:[], activeJobs:0 };
const $ = id => document.getElementById(id);
const modal = $('modal');
const modalBody = $('modal-body');
const shelf = $('shelf');
const number = new Intl.NumberFormat('de-DE',{maximumFractionDigits:2});
const collator = new Intl.Collator('de',{numeric:true,sensitivity:'base'});
const text = (tag, value, className) => {
  const node = document.createElement(tag);
  if (value != null) node.textContent = value;
  if (className) node.className = className;
  return node;
};
const button = (label, action, className) => {
  const node=text('button',label,className);
  node.type='button';
  node.addEventListener('click',action);
  return node;
};
const option = (value,label) => new Option(label,value);
const field = (label, input) => {
  const node=text('label',label);
  node.append(input);
  return node;
};
const input = (type, name, value, placeholder) => {
  const node=document.createElement('input');
  node.type=type; node.name=name; node.value=value ?? '';
  if (placeholder) { node.placeholder=placeholder; node.setAttribute('aria-label',placeholder); }
  return node;
};
const httpsURL = value => {
  try { const link=new URL(value); return link.protocol==='https:' && !link.username && !link.password ? link.href : ''; }
  catch { return ''; }
};
const link = (value,label) => {
  const node=text('a',label);
  node.href=httpsURL(value);
  node.target='_blank'; node.rel='noopener noreferrer';
  return node;
};
async function api(endpoint, method='GET', data) {
  const response = await fetch('api/'+endpoint, {method, headers:data ? {'Content-Type':'application/json'} : {},body:data ? JSON.stringify(data) : undefined});
  if (response.status===204) return;
  const result=await response.json();
  if (!response.ok) throw new Error(result.error || 'Die Anfrage konnte nicht abgeschlossen werden.');
  return result;
}
const route = (endpoint, values) => endpoint+'?'+new URLSearchParams(values);
const average = item => item.ratings?.every(Boolean) ? item.ratings.reduce((sum,value)=>sum+value,0)/6 : null;
const streamQuery = item => /^((movie|series):(?:imdb:tt|tmdb:)[0-9]+)$/.test(item.id);
const current = () => definitions[state.kind];

function cover(item) {
  const letters=item.title.trim().split(/\s+/).slice(0,2).map(word=>word[0]).join('').toUpperCase();
  const node=text('span',letters || definitions[item.kind].icon,'cover');
  node.setAttribute('aria-hidden','true');
  const source=httpsURL(item.cover);
  if (source) {
    const image=document.createElement('img');
    image.src=source; image.alt=''; image.loading='lazy'; image.referrerPolicy='no-referrer';
    image.addEventListener('error',()=>image.remove(),{once:true});
    node.append(image);
  }
  return node;
}

function tabs() {
  $('tabs').setAttribute('role','tablist');
  $('tabs').replaceChildren(...Object.entries(definitions).map(([kind, info])=>{
    const tab=button(info.title,()=>{
      state.kind=kind; state.status=''; state.genre=''; tabs(); render();
    });
    tab.setAttribute('role','tab');
    tab.setAttribute('aria-selected',kind===state.kind);
    return tab;
  }));
}

function render() {
  const info=current();
  const items=state.entries.filter(item=>item.kind===state.kind);
  $('heading').textContent=info.title;
  $('count').textContent=items.length+' '+(items.length===1?'gespeicherter Titel':'gespeicherte Titel');
  $('status-filters').replaceChildren(...[['','Alle'],...Object.entries(info.states)].map(([value,label])=>{
    const node=button(label,()=>{ state.status=value; render(); });
    node.setAttribute('aria-pressed',state.status===value);
    return node;
  }));
  const genres=[...new Set(items.flatMap(item=>item.genres||[]))].sort(collator.compare);
  if (!genres.includes(state.genre)) state.genre='';
  $('genre').replaceChildren(option('','Alle Genres'),...genres.map(genre=>option(genre,genre)));
  $('genre').value=state.genre;
  const visible=items.filter(item=>(!state.status||item.status===state.status)&&(!state.genre||item.genres?.includes(state.genre)));
  visible.sort(compare);
  shelf.replaceChildren(...visible.map(card));
  if (visible.length) return;
  const empty=text('div',null,'empty');
  const body=text('div');
  body.append(text('h2',items.length?'Keine passenden Titel':'Dein Regal ist noch leer'),text('p',items.length?'Wähle einen anderen Status oder ein anderes Genre.':'Finde einen Titel über die Suche oder trage ihn manuell ein.'));
  if (!items.length) body.append(button('＋ Titel hinzufügen',openSearch,'accent'));
  empty.append(body); shelf.append(empty);
}

function compare(a,b) {
  if (state.sort==='best'||state.sort==='worst') {
    const x=average(a), y=average(b);
    if (x===null && y!==null) return 1;
    if (y===null && x!==null) return -1;
    if (x!==null && y!==null && x!==y) return state.sort==='best' ? y-x : x-y;
  }
  if (state.sort==='genre') {
    const order=collator.compare(a.genres?.[0] || '\uffff',b.genres?.[0] || '\uffff');
    if (order) return order;
  }
  return collator.compare(a.title,b.title);
}

function card(item) {
  const article=text('article',null,'card');
  const open=button('',()=>openEditor(item),'card-button');
  const body=text('span',null,'card-content');
  body.append(text('strong',item.title));
  if (item.subtitle) body.append(text('small',item.subtitle));
  const avg=average(item);
  body.append(text('small',avg===null?'Ohne Bewertung':'★ '+number.format(avg)+' / 5',avg===null?'':'score'));
  open.append(cover(item),body);
  const foot=text('div',null,'card-foot');
  foot.append(text('span',current().states[item.status],'chip'));
  if (item.status==='started') foot.append(text('small',' · '+item.progress+(item.total?' / '+item.total:'')+' '+current().unit,'muted'));
  if (item.kind!=='book' && item.status==='want') foot.append(streamPanel(item));
  article.append(open,foot);
  return article;
}

function startJobs() {
  while (state.activeJobs<3 && state.streamJobs.length) {
    const job=state.streamJobs.shift();
    state.activeJobs++;
    api(route('streaming',{id:job.id})).then(job.resolve,job.reject).finally(()=>{
      state.activeJobs--; startJobs();
    });
  }
}

function loadStream(id) {
  const saved=state.streamCache.get(id);
  if (saved && saved.until>Date.now()) return saved.promise;
  const cache={until:Date.now()+600000};
  cache.promise=new Promise((resolve,reject)=>state.streamJobs.push({id,resolve,reject}));
  cache.promise.catch(()=>{ cache.until=Date.now()+60000; });
  state.streamCache.set(id,cache);
  startJobs();
  return cache.promise;
}

function streamPanel(item) {
  const box=text('div',null,'stream');
  const justwatch='https://www.justwatch.com/'+(state.prefs?.region||'DE').toLowerCase()+'/Suche?q='+encodeURIComponent(item.title);
  if (!streamQuery(item) || !(state.prefs?.hasWatchmode || state.prefs?.hasTmdb)) {
    box.append(link(justwatch,'Angebote und Preise bei JustWatch'));
    if (streamQuery(item)) box.append(text('small','Live-Angebote in den Einstellungen aktivieren.'));
    return box;
  }
  box.append(text('small','Streamingangebote werden geladen …'));
  loadStream(item.id).then(data=>{
    box.replaceChildren(text('strong','Streaming · '+data.region));
    if (!data.offers.length) box.append(text('small','Keine gemeldeten Angebote.'));
    const list=text('ul');
    data.offers.slice(0,4).forEach(offer=>list.append(offerLine(offer)));
    box.append(list);
    if (data.offers.length>4) {
      const extra=text('details');
      extra.append(text('summary','Weitere '+(data.offers.length-4)+' Angebote'));
      const remaining=text('ul');
      data.offers.slice(4).forEach(offer=>remaining.append(offerLine(offer)));
      extra.append(remaining); box.append(extra);
    }
    if (data.tmdb) box.append(text('small','Anbieterangaben von JustWatch über TMDB.'));
    if (data.watchmode) box.append(text('small','Streamingdaten von Watchmode. Preis ggf. je Staffel oder Folge.'));
    if (data.partial) box.append(text('small','Eine Datenquelle ist gerade nicht verfügbar.'));
    box.append(link(httpsURL(data.link)||justwatch,'Alle Angebote und Preise ansehen'));
  }).catch(error=>{
    box.replaceChildren(text('small',error.message),link(justwatch,'Angebote und Preise bei JustWatch'));
  });
  return box;
}

function offerLine(offer) {
  const labels={sub:'Im Abo enthalten',free:'Kostenlos',ads:'Kostenlos mit Werbung',rent:'Leihen',buy:'Kaufen',tve:'TV-Anbieter-Zugang'};
  const line=text('li');
  line.append(httpsURL(offer.url)?link(offer.url,offer.name):text('span',offer.name));
  const priced=offer.type==='rent'||offer.type==='buy';
  const amount=offer.price && offer.currency ? new Intl.NumberFormat('de-DE',{style:'currency',currency:offer.currency}).format(offer.price) : '';
  line.append(' · '+labels[offer.type]+(priced ? ' · '+(amount?'ab '+amount:'Preis beim Anbieter') : ''));
  return line;
}

function show(title) {
  const inner=text('section',null,'dialog-inner');
  const header=text('div',null,'dialog-title');
  const close=button('✕',()=>modal.close(),'quiet close');
  close.setAttribute('aria-label','Schließen');
  header.append(text('h2',title),close); inner.append(header);
  modalBody.replaceChildren(inner);
  if (!modal.open) modal.showModal();
  modal.scrollTop=0;
  return inner;
}

function openSearch() {
  const kind=state.kind, info=current();
  const inner=show(info.singular+' hinzufügen');
  const form=text('form',null,'form-line');
  const query=input('search','q','','Titel'+(kind==='book'?' oder ISBN':''));
  form.append(query);
  if (kind==='book') form.append(input('text','author','','Autor'),input('text','publisher','','Verlag'),input('text','volume','','Reihe und Bandnummer'));
  const submit=text('button','Suchen','accent');
  submit.type='submit'; form.append(submit);
  const results=text('div',null,'results');
  results.setAttribute('aria-live','polite');
  inner.append(form,results,button('Manuell hinzufügen',()=>openEditor({id:'manual:'+crypto.randomUUID(),kind,title:query.value.trim(),ratings:[0,0,0]}),'quiet'));
  query.focus();
  form.addEventListener('submit',async event=>{
    event.preventDefault(); submit.disabled=true;
    results.replaceChildren(text('p','Suche läuft …','muted'));
    const values=Object.fromEntries(new FormData(form)); values.kind=kind;
    try {
      const found=await api(route('search',values));
      results.replaceChildren();
      if (!found.length) results.append(text('p','Keine Treffer. Prüfe die Schreibweise oder füge den Titel manuell hinzu.','muted'));
      for (const item of found) {
        const title=text('span');
        title.append(text('strong',item.title),text('small',[item.subtitle,item.source].filter(Boolean).join(' · ')));
        const match=button('',()=>selectResult(item),'result');
        match.append(cover(item),title); results.append(match);
      }
    } catch(error) { results.replaceChildren(text('p',error.message,'error')); }
    finally { submit.disabled=false; }
  });
}

async function selectResult(item) {
  const existing=state.entries.find(saved=>saved.id===item.id);
  if (existing) { openEditor(existing); return; }
  const loading=show(item.title);
  const status=text('p','Lade Details …','muted');
  loading.append(status);
  if (streamQuery(item) || item.id.startsWith('book:ol:')) {
    try {
      const details=await api(route('details',{id:item.id}));
      item={...item,genres:details.genres?.length?details.genres:item.genres,total:details.total || item.total,cover:details.cover||item.cover};
    } catch { /* The search result can still be saved. */ }
  }
  if (modal.open && loading.isConnected) openEditor(item);
}

function stars(item, index, label) {
  const wrap=text('div');
  wrap.append(text('div',label,'rating-label'));
  const row=text('div',null,'stars'); row.setAttribute('role','group'); row.setAttribute('aria-label',label);
  const starsList=[];
  const controls=[];
  const counter=text('output');
  const paint=()=>{
    const rating=item.ratings[index];
    starsList.forEach((fill,i)=>{ fill.className=rating>=i*2+2?'full':'half';fill.hidden=rating<=i*2; });
    controls.forEach((control,i)=>control.setAttribute('aria-pressed',rating===i+1));
    counter.value=rating?number.format(rating/2)+' / 5':'Offen';
  };
  for (let i=0;i<5;i++) {
    const star=text('span','★','star');
    const fill=text('span','★','full'); fill.setAttribute('aria-hidden','true');
    starsList.push(fill);
    star.append(fill);
    for (const value of [i*2+1,i*2+2]) {
      const control=button('',()=>{ item.ratings[index]=value; paint(); });
      control.setAttribute('aria-label',label+' '+number.format(value/2)+' Sterne');
      control.title=number.format(value/2)+' Sterne';
      controls.push(control); star.append(control);
    }
    row.append(star);
  }
  row.append(counter,button('Zurücksetzen',()=>{item.ratings[index]=0;paint();},'clear-stars'));
  wrap.append(row);paint();return wrap;
}

function openEditor(original) {
  const info=definitions[original.kind], item={...original,ratings:[...(original.ratings||[0,0,0])]};
  const existing=state.entries.some(saved=>saved.id===item.id);
  const inner=show(item.title||info.singular+' hinzufügen');
  const form=text('form');
  const detail=text('div',null,'detail');
  detail.append(cover({...item,title:item.title||info.singular}));
  const fields=text('div',null,'edit-fields');
  const title=input('text','title',item.title); title.required=true; title.maxLength=250;
  const subtitle=input('text','subtitle',item.subtitle);
  const coverURL=input('url','cover',item.cover);
  const genre=input('text','genre',(item.genres||[]).join(', '));
  const status=document.createElement('select'); status.name='status';
  for(const [value,label] of Object.entries(info.states)) status.append(option(value,label));
  status.value=item.status || 'want';
  fields.append(field('Titel',title),field(item.kind==='book'?'Autor / Ausgabe':'Jahr / Zusatz',subtitle),field('Genres (durch Komma getrennt)',genre),field('Status',status));
  if (item.id.startsWith('manual:')) fields.append(field('Cover-URL (optional)',coverURL));
  const progress=input('number','progress',item.progress||0);
  const total=input('number','total',item.total||'');
  progress.min='0'; progress.step='1'; total.min='1'; total.step='1';
  const metrics=text('div',null,'progress-fields');
  if (info.unit) {
    metrics.append(field(item.kind==='book'?'Aktuelle Seite':'Aktuelle Folge',progress),field(info.unit+' insgesamt',total));
    fields.append(metrics);
  }
  const viewing=text('div',null,'detail-stream');
  const syncProgress=()=>{
    metrics.hidden=status.value!=='started';
    progress.disabled=status.value!=='started';
    progress.max=total.value||'';
    if (!info.unit) metrics.remove();
  };
  const syncWatch=()=>{
    viewing.replaceChildren();
    if(item.kind!=='book' && status.value==='want' && existing) viewing.append(streamPanel(item));
  };
  status.addEventListener('change',()=>{syncProgress();syncWatch();});
  total.addEventListener('input',syncProgress); syncProgress(); syncWatch();
  const criteria=item.kind==='book'?ratings.book:ratings.screen;
  criteria.forEach((label,index)=>fields.append(stars(item,index,label)));
  fields.append(text('p','Der Durchschnitt erscheint, sobald alle drei Kriterien bewertet sind.','form-note'));
  detail.append(fields); form.append(detail,viewing);
  const err=text('p',null,'error form-error'); err.setAttribute('role','alert');
  const actions=text('div',null,'dialog-actions');
  if (existing) actions.append(button('Entfernen',async()=>{
    if (!confirm('Diesen Titel aus deinem Regal entfernen?')) return;
    try {
      await api('entries','DELETE',{id:item.id});
      state.entries=state.entries.filter(saved=>saved.id!==item.id); modal.close(); render();
    } catch(error) { err.textContent=error.message; }
  },'quiet remove'));
  actions.append(button('Abbrechen',()=>modal.close(),'quiet'));
  const save=text('button','Speichern','accent'); save.type='submit';actions.append(save);form.append(err,actions);inner.append(form);
  form.addEventListener('submit',async event=>{
    event.preventDefault();
    if (!title.value.trim()) { err.textContent='Gib einen Titel ein.'; return; }
    const max=Number(total.value)||0;
    const position=status.value==='started'?Number(progress.value)||0:Math.min(item.progress||0,max||Infinity);
    const updated={...item,title:title.value.trim(),subtitle:subtitle.value.trim(),cover:httpsURL(coverURL.value),genres:[...new Set(genre.value.split(',').map(value=>value.trim()).filter(Boolean))],status:status.value,total:max,progress:position};
    save.disabled=true;
    try {
      await api('entries','PUT',updated);
      state.entries=state.entries.filter(saved=>saved.id!==item.id).concat(updated);
      modal.close();render();
    } catch(error) { err.textContent=error.message; }
    finally { save.disabled=false; }
  });
}

function openSettings() {
  if (!state.prefs) return;
  const inner=show('Einstellungen');
  const form=text('form',null,'settings-form');
  const region=input('text','region',state.prefs.region);region.required=true;region.maxLength=2;region.pattern='[a-zA-Z]{2}';
  const watch=input('password','watch','',state.prefs.hasWatchmode?'Schlüssel gespeichert':'API-Schlüssel');
  const tmdb=input('password','tmdb','',state.prefs.hasTmdb?'Token gespeichert':'API Read Access Token');
  watch.autocomplete='off';tmdb.autocomplete='off';
  const removeWatch=input('checkbox','removeWatch');
  const removeTmdb=input('checkbox','removeTmdb');
  for(const [node,on] of [[removeWatch,state.prefs.hasWatchmode],[removeTmdb,state.prefs.hasTmdb]]) node.disabled=!on;
  form.append(text('p','Version '+state.prefs.version,'muted'),field('Streaming-Land (DE, AT, CH …)',region),
    field('Watchmode für Anbieter und Preise',watch),link('https://api.watchmode.com/requestApiKey/','Watchmode-Schlüssel für dein Land beziehen'));
  if (state.prefs.hasWatchmode) { const label=field('Watchmode-Schlüssel entfernen',removeWatch);label.className='check';form.append(label); }
  form.append(field('TMDB für deutsche Titel und weitere Anbieter',tmdb),link('https://www.themoviedb.org/settings/api','TMDB-API-Zugang öffnen'));
  if (state.prefs.hasTmdb) { const label=field('TMDB-Token entfernen',removeTmdb);label.className='check';form.append(label); }
  form.append(text('p','This product uses the TMDB API but is not endorsed or certified by TMDB. Streamingdaten von Watchmode und JustWatch. Titeldaten auch von Cinemeta, der Deutschen Nationalbibliothek und Open Library.','form-note'));
  const err=text('p',null,'error form-error');err.setAttribute('role','alert');
  const save=text('button','Speichern','accent');save.type='submit';
  form.append(err,save);inner.append(form);
  form.addEventListener('submit',async event=>{
    event.preventDefault();save.disabled=true;
    try {
      await api('settings','PUT',{region:region.value.toUpperCase(),token:tmdb.value,watchmode:watch.value,remove:removeTmdb.checked,removeWatchmode:removeWatch.checked});
      state.prefs=await api('settings');
      state.streamCache.clear();
      modal.close();render();
    } catch(error) { err.textContent=error.message; }
    finally { save.disabled=false; }
  });
}

$('add').addEventListener('click',openSearch);
$('settings').addEventListener('click',openSettings);
$('genre').addEventListener('change',event=>{ state.genre=event.target.value;render(); });
$('sort').addEventListener('change',event=>{ state.sort=event.target.value;render(); });
$('exit').addEventListener('click',async()=>{
  try { await api('exit','POST'); } catch { return; }
  document.body.replaceChildren(text('main','Medienregal ist beendet. Du kannst dieses Fenster schließen.'));
  window.close();
});
Promise.all([api('entries'),api('settings')]).then(([entries,prefs])=>{
  state.entries=entries||[];state.prefs=prefs;tabs();render();
}).catch(error=>{ $('notice').textContent=error.message; });
setInterval(()=>{ if(!modal.open && !document.hidden) render(); },600000);
