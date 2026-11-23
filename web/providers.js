'use strict';

const providerPresets = [
  'Amazon Video','Apple TV','Apple TV+','ARD Mediathek','Crunchyroll','DAZN','Disney+',
  'HBO Max','Joyn','MagentaTV','Netflix','Paramount+','Prime Video','RTL+','WOW',
  'YouTube','ZDF'
];
const providerAliases = new Map([
  ['amazon prime video','prime video'],['amazon prime','prime video'],['prime video amazon','prime video'],
  ['amazon instant video','amazon video'],['amazon video amazon','amazon video'],
  ['apple itunes','apple tv'],['itunes','apple tv'],
  ['disney plus','disney+'],['disneyplus','disney+'],
  ['paramount plus','paramount+'],['rtl plus','rtl+'],['tvnow','rtl+'],
  ['max','hbo max'],['wow tv','wow'],['zdf mediathek','zdf']
]);
const providerVariants = new Map([
  ['netflix basic with ads','netflix'],['netflix standard with ads','netflix'],
  ['netflix standard mit werbung','netflix'],['netflix with ads','netflix'],
  ['amazon prime video with ads','prime video'],['prime video with ads','prime video'],
  ['disney+ with ads','disney+'],['disney+ mit werbung','disney+']
]);

function providerKey(value, type) {
  const compact=value.trim().toLocaleLowerCase('de-DE').normalize('NFC').replace(/\s+/g,' ');
  // Apple uses the same service name for some store and subscription offers.
  if (compact==='apple tv' && type==='sub') return 'apple tv+';
  return providerAliases.get(compact)||providerVariants.get(compact)||compact;
}

function distinctProviders(values) {
  const names=new Map();
  for (const value of values) {
    const key=providerKey(value);
    if (key && !names.has(key)) names.set(key,value.trim());
  }
  return [...names.values()];
}

function selectedProviderOffers(offers) {
  if (!state.prefs?.providers?.length) return offers;
  if (state.status==='want'&&state.onlySubscriptions) offers=offers.filter(offer=>offer.type==='sub');
  const selected=new Set(state.prefs.providers.map(name=>providerKey(name)));
  return offers.filter(offer=>selected.has(providerKey(offer.name,offer.type)));
}

function providerPicker() {
  const previous=state.prefs.providers||[];
  const active=new Set(previous.map(name=>providerKey(name)));
  const fieldset=text('fieldset',null,'provider-picker');
  fieldset.append(text('legend','Meine Streaminganbieter'));
  const grid=text('div',null,'provider-checks');
  const choices=distinctProviders([...providerPresets,...previous]).sort(collator.compare).map(name=>{
    const checkbox=input('checkbox','provider',name);
    checkbox.checked=active.has(providerKey(name));
    grid.append(field(name,checkbox));
    return checkbox;
  });
  const extra=input('text','custom-provider','','Weitere Anbieter, durch Komma getrennt');
  extra.maxLength=500;
  const streamingConfigured=state.prefs.hasWatchmode||state.prefs.hasTmdb;
  fieldset.append(grid,field('Eigene Anbieter',extra),
    text('p','Ohne Auswahl werden alle Angebote angezeigt.'+(streamingConfigured?'':' Für Live-Angebote richte unten Watchmode oder TMDB ein.'),'form-note'));
  return {
    fieldset,
    selected:()=>distinctProviders([...choices.filter(choice=>choice.checked).map(choice=>choice.value),...extra.value.split(',')])
  };
}

function providerFilter() {
  const filter=$('provider-filter');
  const show=(state.kind==='movie'||state.kind==='series') && state.status==='want' && state.prefs?.providers?.length>0;
  filter.hidden=!show;
  const subscription=$('subscription-filter');
  subscription.hidden=!show;
  if (!show) return false;
  const check=$('only-providers');
  const configured=Boolean(state.prefs.hasWatchmode||state.prefs.hasTmdb);
  check.disabled=!configured;
  check.checked=configured && state.onlyProviders;
  filter.title=configured?'':'Für diesen Filter zuerst Watchmode oder TMDB in den Einstellungen einrichten.';
  const abo=$('only-subscriptions');
  abo.disabled=!configured;
  abo.checked=configured&&state.onlySubscriptions;
  subscription.title=filter.title;
  return check.checked||abo.checked;
}

function filterWatchCards(items, cards) {
  const result=text('p',null,'shelf-availability');
  result.setAttribute('role','status');
  shelf.prepend(result);
  let pending=items.length;
  let matching=0;
  let unknown=0;
  const paint=()=>{
    if (pending) {
      result.textContent='Prüfe deine Anbieter · '+(items.length-pending)+' von '+items.length+' Titeln';
    } else if (matching || unknown) {
      const summary=matching?matching+' bei deinen Anbietern':'Keine passenden Angebote gemeldet';
      result.textContent=summary+(unknown?' · '+unknown+' ohne prüfbare Verfügbarkeitsdaten':'');
    } else {
      result.textContent='Für keinen dieser Titel wurde ein Angebot bei deinen Anbietern gemeldet.';
    }
  };
  cards.forEach((card,index)=>{
    card.hidden=true;
    const item=items[index];
    const resolve=offers=>{
      if (!result.isConnected) return;
      pending--;
      if (offers===null) unknown++;
      if (offers?.length) {
        matching++;
        card.hidden=false;
      }
      paint();
    };
    if (!streamQuery(item)) {
      resolve(null);
      return;
    }
    loadStream(catalogID(item)).then(data=>{
      const offers=selectedProviderOffers(data.offers);
      resolve(data.partial && !offers.length?null:offers);
    },()=>resolve(null));
  });
  paint();
}
