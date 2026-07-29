(() => {
  const panels = [...document.querySelectorAll('[data-panel]')];
  const steps = [...document.querySelectorAll('.step')];
  const platformLabel = document.querySelector('#platform-label');
  const destination = document.querySelector('#install-destination');
  const connectionBadge = document.querySelector('#connection-badge');
  const welcomeActionLabel = document.querySelector('#welcome-action-label');
  const runtimeLabel = document.querySelector('#runtime-label');
  const firstCommand = document.querySelector('#first-command');
  const commandHint = document.querySelector('#command-hint');
  const stageAnnouncement = document.querySelector('#stage-announcement');
  const mode = document.body.dataset.mode || 'preview';
  const sessionToken = new URLSearchParams(window.location.search).get('session') || '';
  let runtime = false;
  let simulation = false;
  let runtimePlatform = '';
  let planDigest = '';
  let verified = false;
  const platform = /Win/i.test(navigator.userAgent) ? 'Windows' : /Mac/i.test(navigator.userAgent) ? 'macOS' : 'seu dispositivo';

  platformLabel.textContent = platform;
  destination.textContent = platform === 'Windows' ? '%LOCALAPPDATA%\\BCGOS' : platform === 'macOS' ? '~/Library/Application Support/BCGOS' : '~/.local/share/bcgos';

  function markChecks(status) {
    document.querySelectorAll('.check-item').forEach(item => {
      const isReady = status === 'ready';
      const isSimulated = status === 'simulated';
      item.querySelector('.check-icon').textContent = isReady ? '✓' : isSimulated ? '◇' : '◌';
      item.querySelector('.check-icon').style.color = isReady ? 'var(--teal)' : isSimulated ? 'var(--gold)' : '';
      item.querySelector('strong').textContent = isReady ? 'pronto' : isSimulated ? 'simulado' : 'aguardando';
      item.querySelector('strong').style.color = isReady ? 'var(--teal)' : isSimulated ? 'var(--gold)' : '';
    });
  }

  function showError(panel, message) {
    const error = document.querySelector(`#runtime-error-${panel}`);
    if (!error) return;
    error.hidden = !message;
    error.querySelector('p').textContent = message || '';
  }

  function showStatus(message) {
    const status = document.querySelector('#workspace-status');
    if (!status) return;
    status.hidden = !message;
    status.querySelector('p').textContent = message || '';
  }

  function showRuntimeStage(panel, message) {
    const status = document.querySelector(`#runtime-${panel}-status`);
    if (!status) return;
    status.hidden = !message;
    status.querySelector('p').textContent = message || '';
  }

  function setButtonLabel(button, label) {
    const text = button?.querySelector('span');
    if (text) text.textContent = label;
  }

  function commandForPath(path) {
    const value = String(path || '').trim();
    if (!value) return 'bcgos doctor';
    if (/windows/i.test(runtimePlatform || platform)) {
      return `& "${value.replace(/"/g, '\\"')}" doctor`;
    }
    return `"${value.replace(/"/g, '\\"')}" doctor`;
  }

  function setFirstCommand(path) {
    if (!firstCommand) return;
    firstCommand.textContent = commandForPath(path);
    if (commandHint && path) {
      commandHint.textContent = /windows/i.test(runtimePlatform || platform)
        ? 'Windows · PowerShell · caminho exato do seu perfil · não altera o PATH global.'
        : 'Caminho exato do seu perfil · não altera o PATH global.';
    }
  }

  function setSimulationCommandHint() {
    if (!commandHint) return;
    commandHint.textContent = 'Ensaio técnico: nenhum executável de release foi instalado; o comando real aparece após um release conectado.';
  }

  function setProgress(stage) {
    const order = ['verify', 'install', 'complete'];
    const current = order.indexOf(stage);
    document.querySelectorAll('[data-progress]').forEach(track => {
      track.querySelectorAll('[data-progress-step]').forEach(step => {
        const position = order.indexOf(step.dataset.progressStep);
        step.classList.toggle('is-active', position === current);
        step.classList.toggle('is-done', position < current);
        if (position === current) step.setAttribute('aria-current', 'step');
        else step.removeAttribute('aria-current');
      });
    });
  }

  function requestOptions(method, body) {
    const headers = { 'Content-Type': 'application/json' };
    if (sessionToken) headers['X-Maestro-Session'] = sessionToken;
    return { method, headers, body: body ? JSON.stringify(body) : undefined };
  }

  function updateModeBanner(state) {
    const banner = document.querySelector('#preview-banner');
    if (!banner) return;
    if (state?.mode === 'simulation') {
      banner.hidden = false;
      banner.querySelector('span').textContent = 'ENSAIO TÉCNICO';
      banner.querySelector('p').textContent = 'Este fluxo simula a instalação em uma pasta isolada. Nenhum release assinado será declarado ou publicado.';
      return;
    }
    banner.hidden = true;
  }

  function updateConnectionChrome(state) {
    const connectedMode = state?.mode || 'preview';
    const copy = connectedMode === 'simulation'
      ? { badge: 'ENSAIO TÉCNICO', action: 'Simular instalação', footer: 'ensaio técnico conectado' }
      : connectedMode === 'runtime'
      ? { badge: 'RELEASE CONECTADO', action: 'Instalar no meu perfil', footer: 'instalação conectada' }
      : { badge: 'MODO NÃO CONECTADO', action: 'Abrir fluxo visual', footer: 'modo de apresentação' };
    document.body.dataset.runtimeMode = connectedMode;
    if (connectionBadge) {
      connectionBadge.textContent = copy.badge;
      connectionBadge.dataset.mode = connectedMode;
    }
    if (welcomeActionLabel) welcomeActionLabel.textContent = copy.action;
    if (runtimeLabel) runtimeLabel.textContent = copy.footer;
  }

  function updateFinishCopy() {
    const lead = document.querySelector('#finish-lead');
    if (!lead) return;
    lead.textContent = simulation
      ? 'O ensaio técnico terminou em uma pasta isolada. Abra os dados para conferir o resultado; nenhum release assinado foi instalado.'
      : runtime
      ? 'O Maestro foi instalado no seu perfil. Abra a pasta dele e escolha um workspace de teste quando estiver pronto.'
      : 'Esta é uma prévia visual. Nenhum arquivo foi instalado; no modo real, o Maestro ficará no seu perfil.';
  }

  function show(name, { focusHeading = true } = {}) {
    panels.forEach(panel => {
      const visible = panel.dataset.panel === name;
      panel.hidden = !visible;
      panel.classList.toggle('is-visible', visible);
    });
    const order = ['welcome', 'check', 'install', 'finish'];
    const index = order.indexOf(name);
    steps.forEach((step, position) => {
      const active = position === index;
      step.classList.toggle('is-current', active);
      step.classList.toggle('is-done', position < index);
      step.disabled = position > index;
      if (active) step.setAttribute('aria-current', 'step');
      else step.removeAttribute('aria-current');
    });
    const announcements = {
      welcome: 'Etapa 1 de 4: boas-vindas. Escolha instalar o Maestro no seu perfil.',
      check: 'Etapa 2 de 4: verificação. Confira o release antes de qualquer mudança.',
      install: 'Etapa 3 de 4: instalação. O Maestro ficará no seu espaço de usuário.',
      finish: 'Etapa 4 de 4: pronto. O Maestro está preparado para o primeiro comando.'
    };
    if (stageAnnouncement) stageAnnouncement.textContent = announcements[name] || '';
    if (focusHeading) {
      const heading = document.querySelector(`[data-panel="${name}"] h1`);
      if (heading) window.requestAnimationFrame(() => heading.focus());
    }
    if (name === 'check') setProgress('verify');
    if (name === 'install') setProgress('install');
    if (name === 'finish') setProgress('complete');
    if (name === 'check' && mode === 'preview' && !runtime) {
      window.setTimeout(() => {
        document.querySelectorAll('.check-item').forEach((item, index) => {
          window.setTimeout(() => {
            item.querySelector('.check-icon').textContent = '◇';
            item.querySelector('.check-icon').style.color = 'var(--gold)';
            item.querySelector('strong').textContent = 'simulado';
            item.querySelector('strong').style.color = 'var(--gold)';
          }, index * 320);
        });
      }, 180);
    }
  }

  async function discoverRuntime() {
    updateConnectionChrome();
    try {
      const response = await fetch('/api/state');
      if (!response.ok) return;
      const state = await response.json();
      runtime = true;
      simulation = state.mode === 'simulation';
      document.body.dataset.mode = 'runtime';
      updateConnectionChrome(state);
      updateModeBanner(state);
      updateFinishCopy();
      runtimePlatform = state.platform || '';
      platformLabel.textContent = state.platform || platform;
      destination.textContent = state.managed_root || destination.textContent;
    } catch (_) {
      // Opening index.html directly is a deliberate, non-mutating preview.
    }
  }

  async function verifyRelease() {
    showError('check', '');
    showRuntimeStage('check', 'Conferindo release, assinatura e destino…');
    if (!runtime) {
      show('install');
      showRuntimeStage('check', '');
      return;
    }
    const button = document.querySelector('[data-action="verify"]');
    button.disabled = true;
    setButtonLabel(button, 'Conferindo…');
    try {
      const response = await fetch('/api/verify', requestOptions('POST'));
      const payload = await response.json();
      if (!response.ok) throw new Error(payload.error || 'Não foi possível verificar este release.');
      markChecks(simulation ? 'simulated' : 'ready');
      verified = true;
      planDigest = payload.plan_digest || '';
      setProgress('install');
      show('install');
    } catch (error) {
      showError('check', error.message);
    } finally {
      showRuntimeStage('check', '');
      setButtonLabel(button, 'Verificar release');
      button.disabled = false;
    }
  }

  async function installRelease() {
    showError('install', '');
    showRuntimeStage('install', 'Preparando a instalação no seu perfil…');
    if (!runtime) {
      show('finish');
      showRuntimeStage('install', '');
      return;
    }
    if (!verified) {
      showError('install', 'Verifique o release antes de instalar.');
      showRuntimeStage('install', '');
      return;
    }
    const button = document.querySelector('[data-action="install"]');
    button.disabled = true;
    setButtonLabel(button, 'Instalando…');
    try {
      const response = await fetch('/api/install', requestOptions('POST', { plan_digest: planDigest }));
      const payload = await response.json();
      if (!response.ok) throw new Error(payload.error || 'A instalação foi interrompida com segurança.');
      setProgress('complete');
      show('finish');
      if (simulation) setSimulationCommandHint();
      else setFirstCommand(payload.cli_path);
      showStatus(simulation
        ? `Ensaio concluído. Sandbox de dados: ${payload.data_root}`
        : `Instalação concluída. Dados do usuário: ${payload.data_root}`);
    } catch (error) {
      showError('install', error.message);
    } finally {
      showRuntimeStage('install', '');
      setButtonLabel(button, 'Instalar Maestro');
      button.disabled = false;
    }
  }

  async function openDataFolder() {
    showStatus('');
    if (!runtime) {
      showStatus('Esta é uma prévia visual: nenhum workspace foi instalado. No modo real, este botão abre a pasta de dados do Maestro.');
      return;
    }
    const button = document.querySelector('[data-action="open-data"]');
    button.disabled = true;
    try {
      const response = await fetch('/api/open-data', requestOptions('POST'));
      const payload = await response.json();
      if (!response.ok) throw new Error(payload.error || 'Não foi possível abrir a pasta do Maestro.');
      showStatus(`Pasta aberta: ${payload.path}`);
    } catch (error) {
      showStatus(error.message);
    } finally {
      button.disabled = false;
    }
  }

  async function copyFirstCommand() {
    const command = firstCommand?.textContent || 'bcgos doctor';
    try {
      if (!navigator.clipboard?.writeText) throw new Error('clipboard unavailable');
      await navigator.clipboard.writeText(command);
      showStatus(`Comando copiado: ${command}`);
    } catch (_) {
      showStatus('Não foi possível copiar automaticamente. Selecione o comando e copie manualmente.');
    }
  }

  document.addEventListener('click', async event => {
    const next = event.target.closest('[data-next]');
    const previous = event.target.closest('[data-prev]');
    const action = event.target.closest('[data-action]')?.dataset.action;
    if (next) show(next.dataset.next);
    if (previous) show(previous.dataset.prev);
    if (event.target.closest('.step') && event.target.closest('.step').classList.contains('is-done')) show(event.target.closest('.step').dataset.step);
    if (action === 'help') document.querySelector('#help-modal').showModal();
    if (action === 'close-help') document.querySelector('#help-modal').close();
    if (action === 'details') document.querySelector('#flow-modal').showModal();
    if (action === 'close-flow') {
      document.querySelector('#flow-modal').close();
      show('check', { focusHeading: false });
      document.querySelector('[data-action="verify"]').focus();
    }
    if (action === 'verify') await verifyRelease();
    if (action === 'install') await installRelease();
    if (action === 'open-data') await openDataFolder();
    if (action === 'copy-path') navigator.clipboard?.writeText(destination.textContent);
    if (action === 'copy-command') await copyFirstCommand();
    if (action === 'close') window.close();
  });

  discoverRuntime();
})();
