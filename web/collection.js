'use strict';

const completeStatus = item => ['seen','finished'].includes(item.status)||(item.kind==='book'&&item.status==='read');

function acceptCollection(data) {
  state.entries=data.entries||[];
  state.lists=data.lists||[];
  state.undo=data.undo||'';
}

async function refreshCollection() {
  acceptCollection(await api('collection'));
  render();
}

function collectionTools() {
  const lists=$('lists-filter');
  const quickFilter=(label,active,select)=>{
    const control=button(label,select,'quiet');
    control.setAttribute('aria-pressed',String(active));
    return control;
  };
  lists.replaceChildren(
    quickFilter('Alle Listen',!state.list,()=>{ state.list='';render(); }),
    ...state.lists.map(name=>quickFilter(name,state.list===name,()=>{state.list=name;render();}))
  );
  const favorite=$('favorites-filter');
  favorite.setAttribute('aria-pressed',String(state.onlyFavorites));
  $('view').value=state.view;
  $('undo-collection').disabled=!state.undo;
  $('undo-collection').title=state.undo?'Rückgängig: '+state.undo:'';
}

function listPicker(item) {
  const panel=text('details',null,'genre-picker');
  const summary=text('summary','Eigene Listen');
  panel.append(summary);
  if (!state.lists.length) panel.append(text('p','Lege über „Neue Liste“ in der Übersicht eine Liste an.','form-note'));
  const checks=state.lists.map(name=>{
    const check=input('checkbox','list',name);
    check.checked=item.lists?.includes(name)||false;
    panel.append(field(name,check));
    return check;
  });
  const chosen=()=>checks.filter(check=>check.checked).map(check=>check.value);
  function paint() { summary.textContent=chosen().length?'Eigene Listen · '+chosen().join(', '):'Eigene Listen'; }
  panel.addEventListener('change',paint);
  paint();
  return {panel,chosen};
}

function newCollectionList() {
  const inner=show('Neue Liste');
  const form=text('form',null,'settings-form');
  const name=input('text','name');
  name.required=true;name.maxLength=100;
  const submit=text('button','Liste anlegen','accent');
  submit.type='submit';
  const message=text('p',null,'error form-error');
  message.setAttribute('role','alert');
  form.append(field('Listenname',name),message,submit);
  inner.append(form);
  name.focus();
  form.addEventListener('submit',async event=>{
    event.preventDefault();
    submit.disabled=true;
    try {
      const data=await api('lists','PUT',{name:name.value});
      acceptCollection(data);
      $('notice').textContent='Liste angelegt. Titel kannst du in ihrer Detailansicht zuordnen.';
      modal.close();render();
    } catch(e) { message.textContent=e.message; }
    finally { submit.disabled=false; }
  });
}

async function undoLastCollectionChange() {
  const action=state.undo;
  try {
    acceptCollection(await api('collection/undo','POST',{}));
    render();
    $('notice').textContent=action+' · rückgängig gemacht';
  } catch(e) { $('notice').textContent=e.message; }
}

function matchWords(value) {
  return value.normalize('NFKD').replace(/\p{M}/gu,'').toLowerCase().replace(/[^\p{L}\p{N}]+/gu,' ').trim();
}

function matchingCatalogTitles(item) {
  return [item.title,...(item.aliases||[])].map(matchWords).filter(Boolean);
}

function matchingPairs(items) {
  const byTitle=new Map();
  const seen=new Set();
  const pairs=[];
  for (const item of items) {
    const id=catalogID(item);
    const keys=[...new Set([...matchingCatalogTitles(item),...(!id.startsWith('manual:')?['id:'+id]:[])])];
    for (const key of keys) {
      const earlier=byTitle.get(key)||[];
      for (const first of earlier) {
        const identity=[first.id,item.id].sort().join('\n');
        if (!seen.has(identity)) { seen.add(identity); pairs.push([first,item]); }
        if (pairs.length===50) return pairs;
      }
      earlier.push(item);byTitle.set(key,earlier);
    }
  }
  return pairs;
}

function openDuplicates() {
  const inner=show('Doppelte Titel zusammenführen');
  const items=state.entries.filter(item=>item.kind===state.kind).sort((a,b)=>collator.compare(a.title,b.title));
  if (items.length<2) { inner.append(text('p','Speichere mindestens zwei Titel in diesem Bereich.','form-note'));return; }
  const byID=new Map(items.map(item=>[item.id,item]));
  const form=text('form',null,'settings-form');
  const keep=document.createElement('select');
  const remove=document.createElement('select');
  const pairs=matchingPairs(items);
  const suggestion=document.createElement('select');
  const label=item=>[item.title,item.subtitle].filter(Boolean).join(' · ');
  suggestion.append(option('','Vorschlag wählen'),...pairs.map(([a,b],i)=>option(String(i),label(a)+' ↔ '+label(b))));
  for(const [select,hint] of [[keep,'Titel auswählen'],[remove,'Duplikat auswählen']]) {
    select.append(option('',hint),...items.map(item=>option(item.id,label(item))));
  }
  const sourceFields=[
    ['ratings','Bewertungen',item=>average(item)===null?'Ohne Gesamtbewertung':number.format(average(item))+' / 5'],
    ['progress','Fortschritt',item=>item.progress+(item.total?' / '+item.total:'')],
    ['status','Status',item=>definitions[item.kind].states[item.status]]
  ];
  const choices=sourceFields.map(([name,label,summary])=>({name,label,summary,select:document.createElement('select')}));
  const picker=text('div',null,'settings-form');
  const save=text('button','Zusammenführen','accent');
  save.type='submit';
  const error=text('p',null,'error form-error');
  error.setAttribute('role','alert');
  const sync=()=>{
    const first=byID.get(keep.value),second=byID.get(remove.value);
    picker.hidden=!first||!second||first.id===second.id;
    save.disabled=picker.hidden;
    if (picker.hidden) return;
    for(const choice of choices) {
      choice.select.replaceChildren(...[first,second].map(item=>option(item.id,label(item)+' · '+choice.summary(item))));
    }
  };
  suggestion.addEventListener('change',()=>{
    const pair=pairs[Number(suggestion.value)];
    if (suggestion.value && pair) { keep.value=pair[0].id;remove.value=pair[1].id;sync(); }
  });
  keep.addEventListener('change',sync);
  remove.addEventListener('change',sync);
  if (pairs.length) form.append(field('Mögliche Duplikate',suggestion));
  form.append(field('Diesen Titel behalten',keep),field('Diesen Titel damit verbinden',remove));
  for(const choice of choices) picker.append(field(choice.label+' übernehmen von',choice.select));
  form.append(picker,text('p','Genres, Listen und persönliche Notizen werden zusammengeführt.','form-note'),error,save);
  inner.append(form);sync();
  form.addEventListener('submit',async event=>{
    event.preventDefault();
    save.disabled=true;
    try {
      acceptCollection(await api('collection/merge','POST',{
        keep:keep.value,remove:remove.value,...Object.fromEntries(choices.map(choice=>[choice.name,choice.select.value]))
      }));
      modal.close();render();
      $('notice').textContent='Titel zusammengeführt.';
    } catch(e) { error.textContent=e.message;save.disabled=false; }
  });
}

function collectionStatistics() {
  const panel=$('statistics');
  const year=document.createElement('select');
  year.setAttribute('aria-label','Statistikjahr');
  const known=[...new Set([new Date().getFullYear(),...state.entries.filter(completeStatus).map(item=>item.completedYear).filter(Boolean)])].sort((a,b)=>b-a);
  year.append(option('','Alle Jahre'),...known.map(value=>option(String(value),String(value))));
  year.value=state.statsYear||'';
  year.addEventListener('change',()=>{ state.statsYear=year.value;collectionStatistics(); });
  const title=text('div',null,'stats-title');
  title.append(text('h1','Statistik'),year);
  const completed=state.entries.filter(item=>completeStatus(item)&&(!year.value||Number(year.value)===item.completedYear));
  const counts=text('div',null,'stats-counts');
  for(const [kind,definition] of Object.entries(definitions)) {
    const tile=text('div',null,'stat-tile');
    tile.append(text('strong',completed.filter(item=>item.kind===kind).length),text('span',definition.title+' abgeschlossen'));
    counts.append(tile);
  }
  const undated=state.entries.filter(item=>completeStatus(item)&&!item.completedYear).length;
  const sections=text('div',null,'stats-sections');
  const genrePanel=text('section',null,'stat-tile');
  const genres=new Map();
  for(const item of completed) {
    for(const genre of genreLabels(item.kind,item.genres||[])) genres.set(genre,(genres.get(genre)||0)+1);
  }
  genrePanel.append(text('h2','Häufigste Genres'));
  const popular=text('ul');
  for(const [genre,count] of [...genres].sort((a,b)=>b[1]-a[1]||collator.compare(a[0],b[0])).slice(0,8)) {
    const row=text('li');row.append(text('span',genre),text('strong',count));popular.append(row);
  }
  genrePanel.append(popular.children.length?popular:text('p','Keine Genres für diesen Zeitraum.','form-note'));
  const ratedPanel=text('section',null,'stat-tile');
  ratedPanel.append(text('h2','Beste Bewertungen'));
  const top=text('ul');
  const rated=(year.value?completed:state.entries).filter(item=>average(item)!==null).sort((a,b)=>average(b)-average(a)||collator.compare(a.title,b.title)).slice(0,8);
  for (const item of rated) {
    const row=text('li');row.append(button(item.title,()=>openEditor(item),'quiet'),text('strong','★ '+number.format(average(item))));top.append(row);
  }
  ratedPanel.append(top.children.length?top:text('p','Keine vollständigen Bewertungen für diesen Zeitraum.','form-note'));
  sections.append(genrePanel,ratedPanel);
  panel.replaceChildren(title,counts);
  if (undated) panel.append(text('p',undated+' ältere Abschlüsse ohne Datum sind in „Alle Jahre“ enthalten.','form-note'));
  panel.append(sections);
}

function favoriteButton(item) {
  const control=button(item.favorite?'♥':'♡',async()=>{
    control.disabled=true;
    try {
      acceptCollection(await api('collection/favorite','PUT',{id:item.id,favorite:!item.favorite}));
      render();
      for (const favorite of shelf.querySelectorAll('.favorite-toggle')) {
        if (favorite.dataset.entryId===item.id) { favorite.focus({preventScroll:true});break; }
      }
    } catch(e) {
      $('notice').textContent=e.message;
      control.disabled=false;
    }
  },'favorite-toggle');
  control.dataset.entryId=item.id;
  control.title=item.favorite?'Aus Favoriten entfernen':'Zu Favoriten hinzufügen';
  control.setAttribute('aria-label',control.title+': '+item.title);
  control.setAttribute('aria-pressed',String(Boolean(item.favorite)));
  return control;
}
