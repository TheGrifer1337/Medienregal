'use strict';

let accountStatus={platform:'desktop'};
let syncingCollection=false;
let syncError='';

function mobileMode() {
  return accountStatus.platform==='web';
}

function paintAccount() {
  const button=$('account');
  button.textContent=accountStatus.name||tr('Konto');
  button.setAttribute('aria-label',tr('Konto')+(accountStatus.name?' · '+accountStatus.name:''));
  $('exit').hidden=mobileMode();
  document.body.dataset.platform=accountStatus.platform;
}

function showAccount() {
  const inner=show(tr('Konto'));
  const form=text('form',null,'settings-form account-form');
  const error=text('p',null,'error form-error');
  error.setAttribute('role','alert');
  const summary=text('p',accountStatus.name?tr('Angemeldet als ')+accountStatus.name:tr('Melde dich mit demselben Konto auf Windows und iPhone an.'),'form-note');
  inner.append(summary);
  if (!mobileMode()) {
    const serverBox=text('section',null,'account-server');
    const hostNote=text('p',tr('Du kannst diesen PC als Kontoserver nutzen. Für den Zugriff vom iPhone brauchst du seine HTTPS-Adresse.'),'form-note');
    serverBox.append(hostNote);
    const questions=link('https://github.com/TheGrifer1337/Medienregal#iphone-und-kontoserver',tr('Anleitung für das iPhone öffnen'));
    if (accountStatus.hostActive) {
      const invite=input('text','invite-code',accountStatus.invite);
      invite.readOnly=true;
      invite.addEventListener('focus',()=>invite.select());
      serverBox.append(field(tr('Einladungscode dieses Servers'),invite),questions);
    } else {
      const start=button(tr('Kontoserver auf diesem PC starten'),async()=>{
        start.disabled=true;
        try {
          accountStatus=await api('account','POST',{action:'host'});
          if(inner.isConnected) showAccount();
        } catch(e) {error.textContent=e.message;start.disabled=false;}
      },'quiet');
      serverBox.append(start,questions);
    }
    inner.append(serverBox);
  }
  if (accountStatus.name) {
    inner.append(button(tr('Abmelden'),async()=>{
      try {
        const route=mobileMode()?'account/logout':'account';
        await api(route,'POST',{action:'logout'});
        location.reload();
      } catch(e) {error.textContent=e.message;}
    },'quiet'));
    if (!mobileMode()) {
      const progress=text('section',null,'account-server');
      inner.append(progress);
      api('device-sync').then(state=>{
        if (!progress.isConnected) return;
        progress.append(text('p',state.pending ? state.pending+' '+tr('Änderungen warten auf den Server') : tr('Keine offenen Änderungen'),'form-note'));
        for (const conflict of state.conflicts||[]) {
          const row=text('div',null,'settings-form');
          row.append(text('strong',conflict.current?.title||tr('Titel wurde am Server gelöscht')));
          for (const [label,keepLocal] of [[tr('Meine Änderung anwenden'),true],[tr('Serverstand übernehmen'),false]]) {
            row.append(button(label,async()=>{
              try {
                await api('device-sync','POST',{id:conflict.id,keepLocal});
                await refreshCollection();
                if (inner.isConnected) showAccount();
              } catch(e) {error.textContent=e.message;}
            },'quiet'));
          }
          progress.append(row);
        }
      }).catch(e=>{if (progress.isConnected) progress.textContent=e.message;});
    }
    inner.append(error);
    return;
  }
  const server=input('url','server',accountStatus.server||'','https://…');
  server.required=!mobileMode();
  const action=document.createElement('select');
  action.append(option('login',tr('Anmelden')),option('register',tr('Konto erstellen')));
  const user=input('text','username');
  user.autocomplete='username';user.autocapitalize='none';
  user.required=true;user.pattern='[a-zA-Z0-9][a-zA-Z0-9_.\\-]{2,31}';
  const password=input('password','password');
  password.required=true;password.minLength=10;password.maxLength=128;
  const invite=input('text','invite',accountStatus.invite||'');
  invite.autocapitalize='none';invite.maxLength=64;
  const inviteLabel=field(tr('Einladungscode'),invite);
  const transfer=input('checkbox','transfer');
  transfer.checked=false;
  const transferLabel=field(tr('Meine lokale Sammlung in ein leeres Konto kopieren')+(accountStatus.localCount?' ('+accountStatus.localCount+')':''),transfer);
  const submit=text('button',tr('Anmelden'),'accent');
  submit.type='submit';
  const update=()=>{
    inviteLabel.hidden=action.value!=='register';
    invite.required=action.value==='register';
    password.autocomplete=action.value==='register'?'new-password':'current-password';
    submit.textContent=action.value==='register'?tr('Konto erstellen'):tr('Anmelden');
  };
  action.addEventListener('change',update);
  if (!mobileMode()) form.append(field(tr('Serveradresse'),server));
  form.append(field(tr('Aktion'),action),field(tr('Benutzername'),user),field(tr('Passwort'),password),inviteLabel);
  if (!mobileMode()&&accountStatus.localCount) form.append(transferLabel);
  form.append(error,submit);
  update();
  inner.append(form);
  form.addEventListener('submit',async event=>{
    event.preventDefault();submit.disabled=true;error.textContent='';
    try {
      await api(mobileMode()?'account/session':'account','POST',{
        action:action.value,server:server.value,name:user.value,password:password.value,invite:invite.value,import:transfer.checked,
      });
      location.reload();
    } catch (e) {
      error.textContent=e.message;
      submit.disabled=false;
    }
  });
}

async function startLibrary() {
  accountStatus=await api('account');
  paintAccount();
  if (mobileMode()&&!accountStatus.name) {
    $('collection-panel').hidden=true;
    $('annual-chart').hidden=true;
    $('settings').hidden=true;
    showAccount();
    return;
  }
  const [data,prefs,goals]=await Promise.all([api('collection'),api('settings'),api('goals')]);
  acceptCollection(data);
  state.prefs=prefs;
  state.goals=goals;
  state.view=prefs.view||state.view;
  applyLanguage();
  paintAccount();
  render();
}

async function refreshOnlineLibrary() {
  if (!accountStatus.name || !state.prefs || document.hidden || modal.open || syncingCollection) return;
  syncingCollection=true;
  try {
    const [data,goals]=await Promise.all([api('collection'),api('goals')]);
    if (modal.open || document.hidden) return;
    if (syncError&&$('notice').textContent===syncError) $('notice').textContent='';
    syncError='';
    if (JSON.stringify([data.entries,data.lists,goals,data.undo])!==JSON.stringify([state.entries,state.lists,state.goals,state.undo])) {
      acceptCollection(data);
      state.goals=goals;
      render();
    }
  } catch(e) {
    syncError=e.message;
    $('notice').textContent=syncError;
  } finally {
    syncingCollection=false;
  }
}
