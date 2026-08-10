(() => {
  const screens = [...document.querySelectorAll('[data-screen]')];
  const announcement = document.querySelector('#announcement');
  const failure = document.querySelector('#failure');
  const session = new URLSearchParams(window.location.search).get('session') || '';
  let connected = false;
  let planDigest = '';
  let workspacePath = '';
  let busy = false;

  function options(method, body) {
    const headers = { 'Content-Type': 'application/json' };
    if (session) headers['X-Maestro-Session'] = session;
    return { method, headers, body: body ? JSON.stringify(body) : undefined };
  }

  function show(name) {
    screens.forEach(screen => {
      const active = screen.dataset.screen === name;
      screen.hidden = !active;
      screen.classList.toggle('is-visible', active);
    });
    const heading = document.querySelector(`[data-screen="${name}"] h1`);
    heading?.focus({ preventScroll: true });
  }

  function progress(value, label, phase) {
    const bounded = Math.max(0, Math.min(100, Math.round(value)));
    document.querySelector('#progress-bar').style.width = `${bounded}%`;
    document.querySelector('#progress-value').textContent = `${bounded}%`;
    document.querySelector('#progress-label').textContent = label;
    document.querySelector('.progress-track').setAttribute('aria-valuenow', String(bounded));
    document.querySelectorAll('[data-phase]').forEach(item => item.classList.toggle('is-current', item.dataset.phase === phase));
  }

  function pause(milliseconds) {
    return new Promise(resolve => window.setTimeout(resolve, window.matchMedia?.('(prefers-reduced-motion: reduce)').matches ? 0 : milliseconds));
  }

  function terminalFailure(message) {
    progress(0, 'Não foi possível concluir', 'prepare');
    document.querySelector('#failure-copy').textContent = message || 'Não foi possível concluir a instalação. Nada foi marcado como pronto. Tente novamente.';
    failure.hidden = false;
    busy = false;
  }

  async function response(url, body) {
    const result = await fetch(url, options('POST', body));
    const payload = await result.json().catch(() => ({}));
    if (!result.ok) throw new Error(payload.error || 'Não foi possível concluir a instalação. Nada foi marcado como pronto. Tente novamente.');
    return payload;
  }

  async function prepareWorkspace() {
    // Newer bridges can own this as one opaque simple transaction. Older
    // bridges remain supported through the existing verified transaction.
    const simpleResponse = await fetch('/api/simple-install', options('POST', { plan_digest: planDigest }));
    if (simpleResponse.status !== 404) {
      const simple = await simpleResponse.json().catch(() => ({}));
      if (!simpleResponse.ok) throw new Error(simple.error || 'Não foi possível concluir a instalação. Nada foi marcado como pronto. Tente novamente.');
      workspacePath = simple.workspace_path || '';
      return simple;
    }
    const installed = await response('/api/install', { plan_digest: planDigest });
    progress(78, 'Preparando seu workspace', 'workspace');
    const workspace = await response('/api/create-workspace', { import_existing: false, authorize_setup: true });
    workspacePath = workspace.workspace_path || installed.workspace_path || '';
    return { ...installed, ...workspace };
  }

  async function start(choice) {
    if (busy) return;
    busy = true;
    failure.hidden = true;
    document.querySelectorAll('[data-choice]').forEach(button => button.setAttribute('aria-pressed', String(button.dataset.choice === choice)));
    show('progress');
    progress(8, 'Preparando a instalação', 'prepare');
    document.querySelector('#progress-copy').textContent = choice === 'update' ? 'Estamos atualizando o Maestro.' : 'Estamos instalando o Maestro.';
    try {
      if (!connected) {
        await pause(500);
        progress(100, 'Prévia concluída', 'workspace');
        document.querySelector('#launch-note').textContent = 'Esta é apenas uma prévia. Conecte o instalador para continuar no Claude Desktop.';
        show('complete');
        busy = false;
        return;
      }
      const verified = await response('/api/verify');
      planDigest = verified.plan_digest || '';
      progress(42, 'Preparando o Maestro', 'install');
      await pause(180);
      progress(62, 'Instalando', 'install');
      await prepareWorkspace();
      progress(100, 'Tudo pronto', 'workspace');
      document.querySelector('#launch-note').textContent = '';
      show('complete');
    } catch (error) {
      terminalFailure(error.message);
      return;
    }
    busy = false;
  }

  async function openClaude(button) {
    if (!connected || busy) return;
    busy = true;
    button.disabled = true;
    const label = button.querySelector('span');
    const original = label.textContent;
    label.textContent = 'Abrindo Claude Desktop…';
    try {
      // Existing bridges resolve the freshly prepared workspace server-side.
      // Do not require a newer optional request field to preserve that path.
      await response('/api/launch-runtime', { runtime: 'claude' });
    } catch (_) {
      // Launch is optional to the completed transaction. Keep the completion
      // screen and offer a plain retry instead of turning it into a technical gate.
      document.querySelector('#launch-note').textContent = 'Não conseguimos abrir o Claude Desktop agora. Tente novamente.';
      button.disabled = false;
      label.textContent = original;
    }
    busy = false;
  }

  async function discover() {
    try {
      const state = await fetch('/api/state').then(result => result.ok ? result.json() : null);
      connected = Boolean(state);
      workspacePath = state?.workspace_default || '';
      document.body.dataset.mode = connected ? 'connected' : 'preview';
    } catch (_) {
      // A static file is a safe, non-mutating preview.
    }
  }

  document.addEventListener('click', event => {
    const choice = event.target.closest('[data-choice]');
    if (choice) start(choice.dataset.choice);
    if (event.target.closest('[data-action="retry"]')) start('new');
    const launch = event.target.closest('[data-action="open-claude"]');
    if (launch) openClaude(launch);
  });

  announcement.textContent = 'Escolha uma nova instalação ou uma atualização.';
  discover();
})();
