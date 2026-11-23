'use strict';

function openLauncherImport() {
  const inner=show('Installierte Spiele importieren');
  const intro=text('p','Die Mediathek sucht auf diesem PC nach installierten Spielen aus Steam, Epic Games und der Xbox-App.','form-note');
  const status=text('p','Suche nach installierten Spielen …','form-note');
  status.setAttribute('role','status');
  inner.append(intro,status);
  api('launchers').then(items=>{
    if (!modal.open||!inner.isConnected) return;
    const candidates=items.map(item=>({...item,checked:!item.existing}));
    const available=candidates.filter(item=>!item.existing);
    if (!items.length) {
      status.textContent='Auf diesem PC wurden keine installierten Spiele der drei Launcher erkannt.';
      inner.append(button('Erneut scannen',openLauncherImport,'quiet'));
      return;
    }

    const filters=text('div',null,'history-actions');
    const source=document.createElement('select');
    source.setAttribute('aria-label','Launcher filtern');
    source.append(option('','Alle Launcher'),...['Steam','Epic Games','Xbox'].map(name=>option(name,name)));
    const query=input('search','launcher-search','','Spiel filtern');
    const list=text('div',null,'history-rows launcher-rows');
    const controls=text('div',null,'history-actions');
    const actions=text('div',null,'dialog-actions');
    const error=text('p',null,'error form-error');
    error.setAttribute('role','alert');
    let saving=false;

    const visible=()=>candidates.filter(item=>(!source.value||item.source===source.value)&&
      item.title.toLocaleLowerCase().includes(query.value.trim().toLocaleLowerCase()));
    const save=button('Auswahl importieren',async()=>{
      const chosen=available.filter(item=>item.checked);
      if (!chosen.length||saving) return;
      saving=true;
      save.disabled=true;
      error.textContent='';
      try {
        const result=await api('launchers','POST',{ids:chosen.map(item=>item.id)});
        acceptCollection(result);
        render();
        $('notice').textContent=result.added+' Spiele importiert · '+result.skipped+' Duplikate übersprungen';
        if (inner.isConnected) modal.close();
      } catch(e) {
        error.textContent=e.message;
        saving=false;
        updateCount();
      }
    },'accent');

    function updateCount() {
      const checked=available.filter(item=>item.checked).length;
      status.textContent=items.length+' gefunden · '+(items.length-available.length)+' bereits vorhanden · '+checked+' ausgewählt';
      save.disabled=saving||checked===0;
    }

    function renderRows() {
      list.replaceChildren(...visible().map(item=>{
        const row=text('div',null,'history-row');
        const check=input('checkbox','launcher-game');
        check.checked=item.checked;
        check.disabled=item.existing;
        check.addEventListener('change',()=>{ item.checked=check.checked;updateCount(); });
        row.append(field(item.title,check),text('span',item.source+(item.existing?' · Bereits vorhanden':''),'chip'));
        return row;
      }));
      if (!list.children.length) list.append(text('p','Keine Spiele für diesen Filter.','form-note'));
      updateCount();
    }

    function selectVisible(checked) {
      for (const item of visible()) {
        if (!item.existing) item.checked=checked;
      }
      renderRows();
    }

    source.addEventListener('change',renderRows);
    query.addEventListener('input',renderRows);
    filters.append(source,query);
    controls.append(button('Sichtbare auswählen',()=>selectVisible(true),'quiet'),button('Sichtbare abwählen',()=>selectVisible(false),'quiet'));
    actions.append(button('Erneut scannen',openLauncherImport,'quiet'),button('Abbrechen',()=>modal.close(),'quiet'),save);
    inner.append(filters,controls,list,error,actions);
    renderRows();
  }).catch(error=>{
    if (!inner.isConnected) return;
    status.textContent=error.message;
    inner.append(button('Erneut scannen',openLauncherImport,'quiet'));
  });
}
