'use strict';

function openHistory() {
  const inner=show('Streamingverlauf importieren');
  const form=text('form',null,'settings-form');
  const file=input('file','history-file');
  file.accept='.csv,text/csv';
  const pasted=text('textarea');
  pasted.rows=4;
  pasted.placeholder='Ein Titel pro Zeile';
  pasted.maxLength=4194304;
  const preview=text('button','Vorschau anzeigen','accent');
  preview.type='submit';
  const error=text('p',null,'error form-error');
  error.setAttribute('role','alert');
  form.append(
    text('p','Wähle den Netflix-Titelverlauf als CSV oder füge Titel aus einem anderen Streamingdienst ein. Die Datei wird lokal verarbeitet.','form-note'),
    link('https://help.netflix.com/de/node/101917','Netflix-Titelverlauf herunterladen'),
    field('CSV-Datei',file),
    field('Oder Titel einfügen',pasted),
    error,
    preview
  );
  inner.append(form);
  form.addEventListener('submit',async event=>{
    event.preventDefault();
    const selectedFile=file.files[0];
    const value=pasted.value.trim();
    if ((!selectedFile && !value) || (selectedFile && value)) {
      error.textContent='Wähle eine CSV-Datei oder füge Titel ein.';
      return;
    }
    if (selectedFile?.size>4*1024*1024) {
      error.textContent='Die CSV-Datei darf höchstens 4 MB groß sein.';
      return;
    }
    preview.disabled=true;
    error.textContent='';
    try {
      const format=selectedFile?'csv':'text';
      const body=selectedFile||value;
      const response=await fetch('api/history/preview?format='+format,{method:'POST',body});
      const groups=await response.json();
      if (!response.ok) throw new Error(groups.error);
      if (modal.open && inner.isConnected) showHistoryPreview(groups);
    } catch(e) { error.textContent=e.message; }
    finally { preview.disabled=false; }
  });
}

function showHistoryPreview(groups) {
  const inner=show('Verlauf prüfen');
  const profile=document.createElement('select');
  const chooseProfile=groups.length>1;
  if (chooseProfile) profile.append(option('','Profil auswählen'));
  groups.forEach((group,index)=>profile.append(option(String(index),group.profile||'Ohne Profilnamen')));
  const titleType=document.createElement('select');
  titleType.append(option('movie','Film'),option('series','Serie'));
  const count=text('p',null,'form-note');
  count.setAttribute('role','status');
  const rows=text('div',null,'history-rows');
  const actions=text('div',null,'history-actions');
  const error=text('p',null,'error form-error');
  error.setAttribute('role','alert');
  let selections=[];
  const pageSize=50;
  let page=0;
  const previous=button('Zurück',()=>{page--;paintRows();},'quiet');
  const following=button('Weiter',()=>{page++;paintRows();},'quiet');
  const pageLabel=text('span',null,'form-note');
  const pages=text('div',null,'history-actions');
  pages.append(previous,pageLabel,following);
  const save=button('Auswahl importieren',async()=>{
    const chosen=selections.filter(item=>item.checked).map(item=>({title:item.title,kind:item.kind||titleType.value}));
    if (!chosen.length) return;
    save.disabled=true;
    error.textContent='';
    try {
      const before=new Set(state.entries.map(item=>item.id));
      const result=await api('history/import','POST',chosen);
      await refreshCollection();
      $('notice').textContent=result.added+' hinzugefügt · '+result.updated+' aktualisiert · '+result.skipped+' übersprungen';
      if (inner.isConnected) {
        const added=result.entries.filter(item=>!before.has(item.id)).map(item=>item.id);
        if (added.length) openEnrichment(added);
        else modal.close();
      }
    } catch(e) { error.textContent=e.message; updateCount(); }
  },'accent');
  const updateCount=()=>{
    const selected=selections.filter(item=>item.checked).length;
    count.textContent=selected+' von '+selections.length+' Titeln ausgewählt';
    save.disabled=selected===0;
  };
  const paintRows=()=>{
    const totalPages=Math.ceil(selections.length/pageSize);
    pages.hidden=totalPages<2;
    previous.disabled=page===0;
    following.disabled=page>=totalPages-1;
    pageLabel.textContent='Seite '+(page+1)+' von '+totalPages;
    rows.replaceChildren(...selections.slice(page*pageSize,(page+1)*pageSize).map(item=>{
      const row=text('div',null,'history-row');
      const check=input('checkbox','history-title');
      check.checked=item.checked;
      check.addEventListener('change',()=>{ item.checked=check.checked; updateCount(); });
      const kind=document.createElement('select');
      kind.setAttribute('aria-label','Medientyp für '+item.title);
      kind.append(option('','Standardtyp'),option('movie','Film'),option('series','Serie'));
      kind.value=item.kind;
      kind.addEventListener('change',()=>{ item.kind=kind.value; });
      row.append(field(item.title,check),kind);
      return row;
    }));
    updateCount();
  };
  const groupChanged=()=>{
    const group=profile.value===''?null:groups[Number(profile.value)];
    selections=group?.titles.map(item=>({...item,checked:true}))||[];
    page=0;
    paintRows();
  };
  const select=checked=>{
    for(const item of selections) item.checked=checked;
    paintRows();
  };
  profile.addEventListener('change',groupChanged);
  inner.append(text('p','Filme werden als „Angesehen“, Serien als „Angefangen“ gespeichert. Prüfe den Typ bei Titeln ohne Folgenangabe.','form-note'));
  const settings=text('div',null,'history-settings');
  if (chooseProfile) settings.append(field('Profil',profile));
  settings.append(field('Standardtyp für unklare Titel',titleType));
  inner.append(settings);
  actions.append(button('Alle auswählen',()=>select(true),'quiet'),button('Auswahl aufheben',()=>select(false),'quiet'));
  const footer=text('div',null,'dialog-actions');
  footer.append(button('Andere Datei oder Liste',openHistory,'quiet'),save);
  inner.append(actions,count,rows,pages,error,footer);
  groupChanged();
}

async function openHistoryUndo() {
  const inner=show('Streamingimport rückgängig machen');
  const feedback=text('p','Prüfe importierte Titel …','form-note');
  feedback.setAttribute('role','status');
  inner.append(feedback);
  try {
    const preview=await api('history/undo');
    if (!modal.open || !inner.isConnected) return;
    const actionable=preview.removed+preview.restored;
    const total=preview.removed+' Titel entfernen · '+preview.restored+' auf „Möchte ich sehen“ zurücksetzen';
    feedback.textContent=actionable?total:'Es gibt keine passenden Importeinträge zum Rückgängigmachen.';
    if (preview.mode==='older' && actionable) {
      inner.append(text('p','Bei älteren Importen kann die App nur neu hinzugefügte Titel erkennen. Die Auswahl kann Einträge aus mehreren Verlaufsimporten enthalten.','form-note'));
    }
    if (preview.preserved) {
      inner.append(text('p',preview.preserved+' nachträglich bearbeitete Importeinträge bleiben erhalten.','form-note'));
    }
    if (preview.examples.length) {
      const examples=text('details');
      examples.append(text('summary',actionable>preview.examples.length?'Beispiele anzeigen':'Betroffene Titel anzeigen'));
      const list=text('ul');
      for (const title of preview.examples) list.append(text('li',title));
      examples.append(list);
      inner.append(examples);
    }
    if (!actionable) return;
    const actions=text('div',null,'dialog-actions');
    const confirmUndo=button(preview.mode==='older'?'Importierte Titel entfernen':'Letzten Import rückgängig machen',async()=>{
      confirmUndo.disabled=true;
      try {
        const result=await api('history/undo','POST',{token:preview.token});
        await refreshCollection();
        $('notice').textContent=result.removed+' importierte Titel entfernt · '+result.restored+' Status zurückgesetzt';
        if (inner.isConnected) modal.close();
      } catch(e) {
        feedback.textContent=e.message;
        confirmUndo.disabled=false;
      }
    },'accent');
    actions.append(button('Abbrechen',()=>modal.close(),'quiet'),confirmUndo);
    inner.append(actions);
  } catch(e) { feedback.textContent=e.message; }
}
