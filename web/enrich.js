'use strict';

function openEnrichment(ids) {
  const inner=show(tr('Importierte Titel vervollständigen'));
  const message=text('p',tr('Für den Katalogabgleich werden die Titel an den Katalogdienst gesendet. Eindeutige Treffer werden automatisch ergänzt.'),'form-note');
  const available=state.entries.filter(item=>item.source==='Streamingverlauf'&&!item.catalogId&&(!ids||ids.includes(item.id)));
  const feedback=text('p',available.length+tr(' Titel ohne Katalogzuordnung'),'form-note');
  feedback.setAttribute('role','status');
  inner.append(message,feedback);
  const rows=text('div',null,'results');
  const actions=text('div',null,'dialog-actions');
  let index=0;
  let saved=0;
  let unresolved=0;
  const batch=20;
  const start=button(tr('Katalogabgleich starten'),async()=>{
    start.disabled=true;
    const end=Math.min(index+batch,available.length);
    for(;index<end;index++) {
      if (!modal.open||!inner.isConnected) break;
      feedback.textContent=tr('Abgleich ')+(index+1)+tr(' von ')+available.length+' · '+saved+tr(' ergänzt');
      const item=available[index];
      try {
        const results=await api(route('search',{kind:item.kind,q:item.title}));
        if (!modal.open||!inner.isConnected) break;
        const exact=results.filter(match=>matchingCatalogTitles(match).includes(matchWords(item.title)));
        if (exact.length===1) {
          await saveMatch(item,exact[0]);
          saved++;
        } else {
          unresolved++;
          appendChoices(item,exact.length?exact:results.slice(0,8));
        }
      } catch {
        unresolved++;
        rows.append(text('p',item.title+tr(' · Katalogabgleich fehlgeschlagen'),'form-note'));
      }
    }
    feedback.textContent=saved+tr(' ergänzt · ')+unresolved+' offen';
    start.disabled=index>=available.length;
    start.textContent=tr('Weitere ')+Math.min(batch,available.length-index)+tr(' Titel abgleichen');
    start.hidden=start.disabled;
    render();
  },'accent');
  start.disabled=!available.length;
  const close=button(tr('Schließen'),()=>modal.close(),'quiet');
  actions.append(close,start);
  inner.append(rows,actions);

  function appendChoices(item,candidates) {
    const row=text('section',null,'enrich-result');
    row.append(text('strong',item.title));
    if (!candidates.length) {
      row.append(text('p',tr('Kein Katalogtreffer.'),'form-note'));rows.append(row);return;
    }
    const select=document.createElement('select');
    select.setAttribute('aria-label',tr('Treffer für ')+item.title);
    select.append(option('',tr('Treffer auswählen')),...candidates.map((candidate,i)=>option(String(i),[candidate.title,candidate.subtitle].filter(Boolean).join(' · '))));
    const choose=button(tr('Übernehmen'),async()=>{
      if (select.value==='') return;
      choose.disabled=true;
      try {
        await saveMatch(item,candidates[Number(select.value)]);
        saved++;unresolved--;
        row.remove();feedback.textContent=saved+tr(' ergänzt · ')+unresolved+' offen';
        render();
      } catch(e) { feedback.textContent=e.message;choose.disabled=false; }
    },'quiet');
    row.append(select,choose);rows.append(row);
  }

  async function saveMatch(item,match) {
    let enriched=match;
    try {
      const details=await api(route('details',{id:match.id}));
      enriched={...match, ...details, cover:details.cover||match.cover,genres:details.genres?.length?details.genres:match.genres};
    } catch { /* The search response still contains the title and available cover. */ }
    if (!modal.open || !inner.isConnected) throw new Error(tr('Abgleich beendet.'));
    const result=await api('collection/enrich','PUT',{id:item.id,match:enriched});
    acceptCollection(result);
  }
}
