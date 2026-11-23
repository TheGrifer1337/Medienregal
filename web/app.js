'use strict';

const definitions = {
  movie: { title:'Filme', singular:'Film', icon:'▤', states:{ seen:'Angesehen', want:'Möchte ich sehen', started:'Angefangen' } },
  series: { title:'Serien', singular:'Serie', icon:'▣', unit:'Folgen', states:{ seen:'Gesehen', started:'Angefangen', want:'Möchte ich sehen' } },
  book: { title:'Bücher', singular:'Buch', icon:'▥', unit:'Seiten', states:{ read:'Gelesen', started:'Angefangen', finished:'Beendet', want:'Möchte ich lesen' } },
  game:{ title:'Spiele',singular:'Spiel',icon:'🎮',states:{library:'In Bibliothek',want:'Möchte ich spielen',started:'Spiele ich gerade',finished:'Durchgespielt'} }
};
const ratings = { game:['Qualität','Gameplay','Storyline'], screen:['Qualität','Storyline','Allgemein'], book:['Storyline','Charakterentwicklung','Schreibstil'] };
const state = { kind:'movie', entries:[], status:'', genre:'', onlyFavorites:false, sort:'title', onlyProviders:false, onlySubscriptions:false, lists:[], list:'', view:'large', undo:'', statsYear:'', prefs:null, streamCache:new Map(), streamJobs:[], activeJobs:0 };
const $ = id => document.getElementById(id);
const modal = $('modal');
const modalBody = $('modal-body');
const shelf = $('shelf');
const number = new Intl.NumberFormat('de-DE',{maximumFractionDigits:2});
const collator = new Intl.Collator('de',{numeric:true,sensitivity:'base'});
const screenGenres = [
  'Abenteuer','Action','Animation','Anime','Biografie','Dokumentation','Drama','Familie',
  'Fantasy','Historisch','Horror','Komödie','Krieg','Krimi','Musik','Mystery',
  'Romantik','Sci-Fi','Sport','Thriller','Übernatürlich','Western'
];
const genrePresets = {
  movie:[...screenGenres,'Musical','Kurzfilm'],
  series:[...screenGenres,'Kinderserie','Reality-TV','Sitcom','Soap','Talkshow'],
  game:['Abenteuer','Action','Horror','Indie','Jump ’n’ Run','Mehrspieler','Puzzle','Rennspiel','Rollenspiel','Shooter','Simulation','Sport','Strategie'],
  book:[
    'Abenteuer','Autobiografie','Belletristik','Biografie','Bilderbuch','Comic','Dark Romance',
    'Drama','Dystopie','Fachbuch','Fantasy','Graphic Novel','Historischer Roman','Horror',
    'Humor & Satire','Jugendbuch','Kinder- und Jugendbuch','Kinderbuch','Kochbuch','Komödie',
    'Krimi','Kurzgeschichten','Liebesroman','Lyrik','Manga','Mystery','Ratgeber','Reisebuch',
    'Roman','Romantasy','Sachbuch','Sci-Fi','Thriller'
  ]
};
const genreAliases = new Map([
  ['adventure','Abenteuer'],['comedy','Komödie'],['komoedie','Komödie'],['crime','Krimi'],
  ['documentary','Dokumentation'],['doku','Dokumentation'],['romance','Romantik'],
  ['science fiction','Sci-Fi'],['sci fi','Sci-Fi'],['family','Familie'],['history','Historisch'],['rpg','Rollenspiel'],['racing','Rennspiel'],['strategy','Strategie'],['sports','Sport'],['platformer','Jump ’n’ Run'],
  ['music','Musik'],['war','Krieg'],['supernatural','Übernatürlich'],['graphic novels','Graphic Novel'],
  ['dark romence','Dark Romance'],['dark romance','Dark Romance'],['nonfiction','Sachbuch'],
  ['non fiction','Sachbuch'],['fiction','Belletristik'],['novel','Roman']
]);
const combinedScreenGenres = new Map([
  ['action & adventure',['Action','Abenteuer']],
  ['action & abenteuer',['Action','Abenteuer']],
  ['science fiction & fantasy',['Sci-Fi','Fantasy']],
  ['sci fi & fantasy',['Sci-Fi','Fantasy']],
  ['war & politics',['Krieg','Politik']],
  ['krieg & politik',['Krieg','Politik']]
]);
const broadBookGenres = new Set(['Belletristik','Roman','Kinder- und Jugendbuch','Kinderbuch','Jugendbuch']);

function genreKey(value) {
  return value.trim().toLocaleLowerCase('de-DE').normalize('NFC').replace(/[‐‑‒–—-]/g,' ').replace(/\s+/g,' ');
}

function genreLabels(kind, values) {
  const labels=new Map();
  const expanded=kind==='book'?values:values.flatMap(value=>combinedScreenGenres.get(genreKey(value))||[value]);
  for (const value of expanded) {
    const key=genreKey(value);
    if (!key) continue;
    let label=genreAliases.get(key) || genrePresets[kind].find(candidate=>genreKey(candidate)===key) || value.trim();
    if (kind==='book' && label==='Romantik') label='Liebesroman';
    const normalized=genreKey(label);
    if (!labels.has(normalized)) labels.set(normalized,label);
  }
  return [...labels.values()];
}

function availableGenres(kind, extra=[]) {
  const saved=state.entries.filter(item=>item.kind===kind).flatMap(item=>item.genres||[]);
  return genreLabels(kind,[...genrePresets[kind],...extra,...saved]).sort(collator.compare);
}

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
  input.removeAttribute('aria-label');
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
const isScreen = item => item.kind==='movie'||item.kind==='series';
const streamQuery = item => /^((movie|series):(?:imdb:tt|tmdb:)[0-9]+)$/.test(catalogID(item));
const current = () => definitions[state.kind];

function cover(item) {
  const letters=item.title.trim().split(/\s+/).slice(0,2).map(word=>word[0]).join('').toUpperCase();
  const node=text('span',letters || definitions[item.kind].icon,'cover');
  node.setAttribute('aria-hidden','true');
  const source=coverSource(item);
  if (source) {
    const image=document.createElement('img');
    image.src=source; image.alt=''; image.loading='lazy'; image.referrerPolicy='no-referrer';
    image.addEventListener('error',()=>image.remove(),{once:true});
    node.append(image);
  }
  return node;
}

function switchCategory(kind) {
  if (kind===state.kind) return;
  state.kind=kind;
  state.status='';
  state.genre='';
  state.onlyFavorites=false;
  document.body.dataset.category=kind;
  for (const tab of $('tabs').querySelectorAll('button')) {
    tab.setAttribute('aria-selected',tab.dataset.kind===kind);
  }
  render();
  if (kind==='stats') return;
  for (const panel of [document.querySelector('.hero-content'),shelf]) {
    panel.classList.remove('category-in');
    void panel.offsetWidth;
    panel.classList.add('category-in');
  }
}

function tabs() {
  $('tabs').setAttribute('role','tablist');
  $('tabs').replaceChildren(...Object.entries({...definitions,stats:{title:'Statistik',icon:'▥'}}).map(([kind, info])=>{
    const tab=button('',()=>switchCategory(kind));
    const icon=text('span',info.icon,'tab-icon');
    icon.setAttribute('aria-hidden','true');
    tab.append(icon,info.title);
    tab.dataset.kind=kind;
    tab.setAttribute('role','tab');
    tab.setAttribute('aria-controls',kind==='stats'?'statistics':'collection-panel');
    tab.setAttribute('aria-selected',kind===state.kind);
    return tab;
  }));
}

function render() {
  const stats=state.kind==='stats';
  $('collection-panel').hidden=stats;
  $('statistics').hidden=!stats;
  annualChart();
  if (stats) { collectionStatistics();return; }
  collectionTools();
  shelf.dataset.view=state.view;
  const info=current();
  const items=state.entries.filter(item=>item.kind===state.kind);
  $('heading').textContent=info.title;
  $('launcher-import').hidden=state.kind!=='game';
  $('count').textContent=items.length+' '+(items.length===1?'gespeicherter Titel':'gespeicherte Titel');
  $('status-filters').replaceChildren(...[['','Alle'],...Object.entries(info.states)].map(([value,label])=>{
    const node=button(label,()=>{ state.status=value; render(); });
    node.setAttribute('aria-pressed',state.status===value);
    return node;
  }));
  const genres=availableGenres(state.kind);
  if (!genres.some(label=>genreKey(label)===state.genre)) state.genre='';
  $('genre').replaceChildren(option('','Alle Genres'),...genres.map(genre=>option(genreKey(genre),genre)));
  $('genre').value=state.genre;
  const visible=items.filter(item=>(!state.onlyFavorites||item.favorite)&&(!state.list||item.lists?.includes(state.list))&&(!state.status||item.status===state.status)&&(!state.genre||genreLabels(item.kind,item.genres||[]).some(genre=>genreKey(genre)===state.genre)));
  visible.sort(compare);
  const onlyProviders=providerFilter();
  const cards=visible.map(card);
  shelf.replaceChildren(...cards);
  if (visible.length) {
    if (onlyProviders) filterWatchCards(visible,cards);
    return;
  }
  const empty=text('div',null,'empty');
  const body=text('div');
  const symbol=text('span',info.icon,'empty-symbol');
  symbol.setAttribute('aria-hidden','true');
  body.append(symbol,text('h2',items.length?'Keine passenden Titel':'Dein Regal ist noch leer'),text('p',items.length?'Wähle eine andere Liste, einen anderen Status oder ein anderes Genre.':'Finde einen Titel über die Suche oder trage ihn manuell ein.'));
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
    const order=collator.compare(genreLabels(a.kind,a.genres||[])[0] || '\uffff',genreLabels(b.kind,b.genres||[])[0] || '\uffff');
    if (order) return order;
  }
  return collator.compare(a.title,b.title);
}

function card(item) {
  const article=text('article',null,'card');
  const open=button('',()=>{
    if (item.kind==='book' && !item.genres?.length && !item.id.startsWith('manual:')) selectResult(item);
    else openEditor(item);
  },'card-button');
  const body=text('span',null,'card-content');
  body.append(text('strong',item.title));
  if (item.subtitle) body.append(text('small',item.subtitle));
  const avg=average(item);
  body.append(text('small',avg===null?'Ohne Bewertung':'★ '+number.format(avg)+' / 5',avg===null?'':'score'));
  open.append(cover(item),body);
  const foot=text('div',null,'card-foot');
  foot.append(text('span',current().states[item.status],'chip'));
  if (item.kind==='game'&&item.source) foot.append(text('small',' · '+item.source,'muted'));
  if (item.status==='started' && current().unit) foot.append(text('small',' · '+item.progress+(item.total?' / '+item.total:'')+' '+current().unit,'muted'));
  if (item.kind==='book' && item.genres?.length) foot.append(text('small',genreLabels(item.kind,item.genres)[0],'book-genre'));
  if (item.lists?.length) foot.append(text('small',item.lists.join(' · '),'book-genre'));
  if (isScreen(item) && item.status==='want') foot.append(streamPanel(item));
  article.append(open,favoriteButton(item),foot);
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
  loadStream(catalogID(item)).then(data=>{
    if (!box.isConnected) return;
    const offers=selectedProviderOffers(data.offers);
    const personal=state.prefs?.providers?.length>0;
    box.replaceChildren(text('strong','Streaming · '+data.region));
    if (!offers.length) box.append(text('small',personal?'Keine Angebote bei deinen Anbietern gemeldet.':'Keine gemeldeten Angebote.'));
    const list=text('ul');
    offers.slice(0,4).forEach(offer=>list.append(offerLine(offer)));
    box.append(list);
    if (offers.length>4) {
      const extra=text('details');
      extra.append(text('summary','Weitere '+(offers.length-4)+' Angebote'));
      const remaining=text('ul');
      offers.slice(4).forEach(offer=>remaining.append(offerLine(offer)));
      extra.append(remaining); box.append(extra);
    }
    if (data.tmdb) box.append(text('small','Anbieterangaben von JustWatch über TMDB.'));
    if (data.watchmode) box.append(text('small','Streamingdaten von Watchmode. Preis ggf. je Staffel oder Folge.'));
    if (data.partial) box.append(text('small','Eine Datenquelle ist gerade nicht verfügbar.'));
    box.append(link(httpsURL(data.link)||justwatch,personal?'Auch andere Anbieter auf JustWatch prüfen':'Alle Angebote und Preise ansehen'));
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
  syncTicker();
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
  if (kind==='movie'||kind==='series') inner.append(button('Streamingverlauf importieren',openHistory,'quiet search-import'));
  if (kind==='game') inner.append(button('Installierte Spiele importieren',openLauncherImport,'quiet search-import'));
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
        title.append(text('strong',item.title),text('small',[item.subtitle,item.kind==='book'?item.genres?.[0]:null,item.source].filter(Boolean).join(' · ')));
        const match=button('',()=>selectResult(item),'result');
        match.append(cover(item),title); results.append(match);
      }
    } catch(error) { results.replaceChildren(text('p',error.message,'error')); }
    finally { submit.disabled=false; }
  });
}

async function selectResult(item) {
  const existing=state.entries.find(saved=>catalogID(saved)===item.id);
  if (existing && (existing.kind!=='book' || existing.genres?.length)) { openEditor(existing); return; }
  item=existing || item;
  const loading=show(item.title);
  const status=text('p','Lade Details …','muted');
  loading.append(status);
  const currentGenres=genreLabels(item.kind,item.genres||[]);
  const isBook=item.kind==='book';
  const needsGenre=isBook && currentGenres.every(genre=>broadBookGenres.has(genre));
  const isbn=(item.cover||'').match(/^https:\/\/covers\.openlibrary\.org\/b\/isbn\/([0-9X]+)-[SML]\.jpg/)?.[1];
  const lookup=streamQuery(item) || /^game:(steam:[0-9]+|wikidata:Q[0-9]+)$/.test(catalogID(item)) || item.id.startsWith('book:ol:') ||
    (needsGenre && (item.id.startsWith('book:open:') || (item.id.startsWith('book:dnb:') && isbn)));
  if (lookup) {
    const params={id:catalogID(item)};
    if (needsGenre) params.genres='1';
    if (isbn) params.isbn=isbn;
    try {
      const details=await api(route('details',params));
      const incoming=details.genres||[];
      const genres=isBook && needsGenre && incoming.length ?
        genreLabels(item.kind,[...incoming.filter(genre=>!broadBookGenres.has(genre)),...currentGenres,...incoming]) :
        (incoming.length?incoming:currentGenres);
      item={...item,genres,total:details.total||item.total,cover:details.cover||item.cover,description:details.description||item.description,aliases:details.aliases||item.aliases};
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

function genrePicker(item) {
  const initial=genreLabels(item.kind,item.genres||[]);
  const isBook=item.kind==='book';
  const panel=text('details',null,'genre-picker');
  const summary=text('summary');
  const list=text('div',null,'genre-checks');
  const names=availableGenres(item.kind,initial);
  const extra=input('text','genre','','z. B. Steampunk, Urban Fantasy');
  const choices=names.map(name=>{
    const check=input('checkbox','genre-choice',name);
    check.checked=initial.some(genre=>genreKey(genre)===genreKey(name));
    const label=field(name,check);
    list.append(label);
    return check;
  });
  const orderedChoices=()=> {
    const checked=choices.filter(check=>check.checked).map(check=>check.value);
    const keys=new Set(checked.map(genreKey));
    return [...initial.filter(name=>keys.has(genreKey(name))),...checked];
  };
  const selected=()=>genreLabels(item.kind,[
    ...orderedChoices(),
    ...extra.value.split(',')
  ]);
  const primary=document.createElement('select');
  primary.name='primary-genre';
  let preferred=initial[0]||'';
  const update=()=>{
    const genres=selected();
    summary.textContent=genres.length?'Genres · '+genres.join(', '):'Genres auswählen';
    if (isBook) {
      primary.replaceChildren(option('','Genre auswählen'),...genres.map(name=>option(name,name)));
      primary.value=genres.includes(preferred)?preferred:(genres[0]||'');
    }
  };
  primary.addEventListener('change',()=>{ preferred=primary.value; });
  list.addEventListener('change',update);
  extra.addEventListener('input',update);
  panel.append(summary,list,field('Eigene Genres (durch Komma getrennt)',extra));
  update();
  return {
    panel,
    primary:isBook?field('Hauptgenre',primary):null,
    genres:()=> {
      const values=selected();
      return primary.value ? [primary.value,...values.filter(name=>name!==primary.value)] : values;
    },
    focus:()=>{ panel.open=true; summary.focus(); }
  };
}

function openEditor(original) {
  const info=definitions[original.kind], item={...original,ratings:[...(original.ratings||[0,0,0])]};
  const existing=state.entries.some(saved=>saved.id===item.id);
  const inner=show(item.title||info.singular+' hinzufügen');
  const form=text('form');
  const detail=text('div',null,'detail');
  const art=text('div',null,'editor-art');
  const previewCover=customCover=>art.replaceChildren(cover({...item,customCover,title:item.title||info.singular}));
  previewCover(item.customCover);
  const coverChoice=customCoverEditor(item,previewCover);
  detail.append(art);
  const fields=text('div',null,'edit-fields');
  const title=input('text','title',item.title); title.required=true; title.maxLength=250;
  const subtitle=input('text','subtitle',item.subtitle);
  const coverURL=input('url','cover',item.cover);
  const genre=genrePicker(item);
  const lists=listPicker(item);
  const notes=text('textarea');
  notes.name='notes';notes.rows=3;notes.maxLength=4000;notes.value=item.notes||'';
  const status=document.createElement('select'); status.name='status';
  for(const [value,label] of Object.entries(info.states)) status.append(option(value,label));
  status.value=item.status || 'want';
  fields.append(field('Titel',title),field(item.kind==='book'?'Autor / Ausgabe':'Jahr / Zusatz',subtitle),genre.panel);
  if (genre.primary) fields.append(genre.primary);
  const favorite=input('checkbox','favorite');
  favorite.checked=Boolean(item.favorite);
  const favoriteField=field('Als Favorit markieren',favorite);
  favoriteField.className='favorite-field';
  fields.append(field('Status',status),favoriteField,coverChoice.section,lists.panel,field('Persönliche Notizen',notes));
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
    if(isScreen(item) && status.value==='want' && existing) viewing.append(streamPanel(item));
  };
  status.addEventListener('change',()=>{syncProgress();syncWatch();});
  total.addEventListener('input',syncProgress); syncProgress(); syncWatch();
  const criteria=ratings[item.kind]||ratings.screen;
  criteria.forEach((label,index)=>fields.append(stars(item,index,label)));
  fields.append(text('p','Der Durchschnitt erscheint, sobald alle drei Kriterien bewertet sind.','form-note'));
  detail.append(fields); form.append(detail,viewing);
  const err=text('p',null,'error form-error'); err.setAttribute('role','alert');
  const actions=text('div',null,'dialog-actions');
  if (existing) actions.append(button('Entfernen',async()=>{
    if (!confirm('Diesen Titel aus deinem Regal entfernen?')) return;
    try {
      await api('entries','DELETE',{id:item.id});
      await refreshCollection();modal.close();
      $('notice').textContent='Titel entfernt. Über „Rückgängig“ kannst du ihn wiederherstellen.';
    } catch(error) { err.textContent=error.message; }
  },'quiet remove'));
  actions.append(button('Abbrechen',()=>modal.close(),'quiet'));
  const save=text('button','Speichern','accent'); save.type='submit';actions.append(save);form.append(err,actions);inner.append(form);
  form.addEventListener('submit',async event=>{
    event.preventDefault();
    if (!title.value.trim()) { err.textContent='Gib einen Titel ein.'; return; }
    const genres=genre.genres();
    if (item.kind==='book' && !genres.length) {
      err.textContent='Wähle mindestens ein Genre für dieses Buch.';
      genre.focus();
      return;
    }
    if (coverChoice.loading()) { err.textContent='Warte kurz, bis das Bild vorbereitet ist.';return; }
    const customCover=coverChoice.value();
    if (customCover && !coverSource({customCover})) { err.textContent='Gib eine HTTPS-Bildadresse ein.';return; }
    const max=Number(total.value)||0;
    const position=status.value==='started'?Number(progress.value)||0:Math.min(item.progress||0,max||Infinity);
    const updated={...item,title:title.value.trim(),subtitle:subtitle.value.trim(),cover:httpsURL(coverURL.value),customCover,favorite:favorite.checked,genres,lists:lists.chosen(),notes:notes.value.trim(),status:status.value,total:max,progress:position};
    save.disabled=true;
    try {
      await api('entries','PUT',updated);
      await refreshCollection();
      modal.close();
    } catch(error) { err.textContent=error.message; }
    finally { save.disabled=false; }
  });
}

function updateControls() {
  const controls=text('section',null,'update-controls');
  const message=text('p',null,'form-note');
  message.setAttribute('role','status');
  let timer;
  const check=button('Nach Updates suchen',()=>refresh('POST'),'quiet');
  check.disabled=true;
  const refresh=async (method='GET')=>{
    clearTimeout(timer);
    if (!controls.isConnected || !modal.open) return;
    if (method==='POST') { check.disabled=true; message.textContent='Suche nach Updates …'; }
    try {
      const status=await api('update',method);
      const messages={
        current:'Version '+status.version+' ist aktuell.',
        checking:'Suche nach Updates …',
        downloading:'Version '+status.version+' wird heruntergeladen …',
        ready:'Version '+status.version+' ist bereit. Beim Beenden wird das Update installiert.'
      };
      const waiting=status.state==='checking'||status.state==='downloading';
      message.textContent=messages[status.state]||status.message||'';
      check.disabled=waiting||status.state==='ready'||status.state==='unavailable';
      if (waiting) timer=setTimeout(refresh,650);
    } catch(e) {
      message.textContent=e.message;
      check.disabled=false;
    }
  };
  controls.append(text('p','Installierte Version '+state.prefs.version,'muted'),check,message);
  return {controls,refresh};
}

function openSettings() {
  if (!state.prefs) return;
  const inner=show('Einstellungen');
  const update=updateControls();
  const imports=text('div',null,'history-actions');
  imports.append(button('Streamingverlauf importieren',openHistory,'quiet'),
    button('Streamingimport rückgängig machen',openHistoryUndo,'quiet'),
    button('Importierte Titel vervollständigen',()=>openEnrichment(),'quiet'));
  inner.append(update.controls,imports);
  update.refresh();
  const form=text('form',null,'settings-form');
  const providers=providerPicker();
  const region=input('text','region',state.prefs.region);region.required=true;region.maxLength=2;region.pattern='[a-zA-Z]{2}';
  const watch=input('password','watch','',state.prefs.hasWatchmode?'Schlüssel gespeichert':'API-Schlüssel');
  const tmdb=input('password','tmdb','',state.prefs.hasTmdb?'Token gespeichert':'API Read Access Token');
  watch.autocomplete='off';tmdb.autocomplete='off';
  const removeWatch=input('checkbox','removeWatch');
  const removeTmdb=input('checkbox','removeTmdb');
  for(const [node,on] of [[removeWatch,state.prefs.hasWatchmode],[removeTmdb,state.prefs.hasTmdb]]) node.disabled=!on;
  form.append(field('Streaming-Land (DE, AT, CH …)',region),providers.fieldset,
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
      await api('settings','PUT',{region:region.value.toUpperCase(),token:tmdb.value,watchmode:watch.value,remove:removeTmdb.checked,removeWatchmode:removeWatch.checked,providers:providers.selected()});
      state.prefs=await api('settings');
      state.streamCache.clear();
      charts.clear();ticker.key='';
      modal.close();render();
    } catch(error) { err.textContent=error.message; }
    finally { save.disabled=false; }
  });
}

document.addEventListener('click', event=>{
  if (!window.chrome?.webview || !event.isTrusted) return;
  const anchor=event.target.closest('a[href]');
  if (!anchor) return;
  const address=httpsURL(anchor.href);
  if (!address) return;
  event.preventDefault();
  api('open-link','POST',{url:address}).catch(error=>{ $('notice').textContent=error.message; });
});
$('add').addEventListener('click',openSearch);
$('launcher-import').addEventListener('click',openLauncherImport);
$('settings').addEventListener('click',openSettings);
$('only-providers').addEventListener('change',event=>{ state.onlyProviders=event.target.checked;render(); });
$('only-subscriptions').addEventListener('change',event=>{ state.onlySubscriptions=event.target.checked;render(); });
$('favorites-filter').addEventListener('click',()=>{state.onlyFavorites=!state.onlyFavorites;render();});
$('new-list').addEventListener('click',newCollectionList);
$('duplicates').addEventListener('click',openDuplicates);
$('undo-collection').addEventListener('click',undoLastCollectionChange);
$('view').addEventListener('change',async event=>{
  const view=event.target.value;
  try { await api('view','PUT',{view});state.view=view;render(); }
  catch(e) { $('notice').textContent=e.message;event.target.value=state.view; }
});
$('genre').addEventListener('change',event=>{ state.genre=event.target.value;render(); });
$('sort').addEventListener('change',event=>{ state.sort=event.target.value;render(); });
$('exit').addEventListener('click',async()=>{
  try { await api('exit','POST'); } catch { return; }
  document.body.replaceChildren(text('main','Die Mediathek ist beendet.'));
  window.close();
});
initChart();
Promise.all([api('collection'),api('settings')]).then(([data,prefs])=>{
  acceptCollection(data);state.prefs=prefs;state.view=prefs.view||state.view;tabs();render();
}).catch(error=>{ $('notice').textContent=error.message; });
setInterval(()=>{ if(!modal.open && !document.hidden) render(); },600000);
