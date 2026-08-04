(() => {
  const panels = [...document.querySelectorAll('[data-panel]')];
  const steps = [...document.querySelectorAll('.step')];
  const platformLabel = document.querySelector('#platform-label');
  const destination = document.querySelector('#install-destination');
  const connectionBadge = document.querySelector('#connection-badge');
  const welcomeActionLabel = document.querySelector('#welcome-action-label');
  const runtimeLabel = document.querySelector('#runtime-label');
  const firstCommand = document.querySelector('#first-command');
  const workspaceDefault = document.querySelector('#workspace-default');
  const commandHint = document.querySelector('#command-hint');
  const stageAnnouncement = document.querySelector('#stage-announcement');
  const mode = document.body.dataset.mode || 'preview';
  const sessionToken = new URLSearchParams(window.location.search).get('session') || '';
  let runtime = false;
  let simulation = false;
  let runtimePlatform = '';
  let planDigest = '';
  let verified = false;
  let installed = false;
  let installedCLIPath = '';
  let runtimeTargets = [];
  let defaultWorkspace = '';
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
    else if (button) button.textContent = label;
  }

  function setWorkspaceProvisioning(phase) {
    const scene = document.querySelector('#workspace-provisioning');
    const setup = document.querySelector('#workspace-setup');
    if (!scene || !setup) return;
    const copy = {
      creating: ['MAESTRO EM MOVIMENTO', 'Criando seu espaço local', 'Montando estrutura, hooks Claude e manutenção no seu perfil.'],
      ready: ['ESPAÇO PREPARADO', 'Seu palco está pronto', 'Workspace, hooks Claude e manutenção local foram configurados.'],
    }[phase];
    if (!copy) {
      scene.hidden = true;
      setup.classList.remove('is-provisioning');
	  document.body.classList.remove('is-workspace-provisioning');
      return;
    }
    document.querySelector('#workspace-provisioning-kicker').textContent = copy[0];
    document.querySelector('#workspace-provisioning-title').textContent = copy[1];
    document.querySelector('#workspace-provisioning-copy').textContent = copy[2];
    scene.dataset.phase = phase;
    scene.hidden = false;
    setup.classList.add('is-provisioning');
	  document.body.classList.add('is-workspace-provisioning');
  }

  function pause(milliseconds) {
    return new Promise(resolve => window.setTimeout(resolve, milliseconds));
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
    const copy = state?.installed
      ? { badge: 'MAESTRO INSTALADO', action: 'Abrir Maestro', footer: 'instalação já concluída' }
      : connectedMode === 'simulation'
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
    const lead = document.querySelector('#runtime-handoff .lead');
    if (!lead) return;
    lead.textContent = simulation
      ? 'O ensaio técnico terminou em uma pasta isolada. Nenhum release assinado foi instalado.'
      : installed || runtime
      ? 'O Maestro foi instalado no seu perfil. Agora, abra um workspace no runtime em que você trabalha.'
      : 'Esta é uma prévia visual. Nenhum arquivo foi instalado; no modo real, o Maestro ficará no seu perfil.';
  }

  function showRuntimeHandoff() {
    document.querySelector('#workspace-setup').hidden = true;
    document.querySelector('#runtime-handoff').hidden = false;
    updateFinishCopy();
  }

  function renderActivation(activation) {
    if (!activation) return;
    let summary = document.querySelector('#activation-summary');
    if (!summary) {
      summary = document.createElement('div');
      summary.id = 'activation-summary';
      summary.className = 'activation-summary';
      summary.innerHTML = '<article><span>HOOKS DO WORKSPACE</span><strong id="activation-hooks">CONFIGURADO</strong><small id="activation-hook-events"></small></article><article><span>REVISÃO DOS HOOKS</span><strong id="activation-hook-review">REVISÃO DO OWNER NECESSÁRIA</strong><small id="activation-hook-review-detail">O Codex pede a confirmação dos comandos locais antes da primeira execução. O instalador não grava confiança global em seu nome.</small></article><article><span>MANUTENÇÃO LOCAL</span><strong id="activation-maintenance">OBSERVADO NO LAUNCHD</strong><small id="activation-maintenance-detail"></small></article><article><span>SESSÃO NATIVA CODEX</span><strong id="activation-native-session">AGUARDANDO PRIMEIRA SESSÃO</strong><small>Configuração não é evidência de que o runtime já executou os hooks.</small></article><article><span>JOBS COM MODELO</span><strong id="activation-model">INDISPONÍVEL</strong><small>Nenhum modelo será executado pela manutenção agendada.</small></article>';
      document.querySelector('#runtime-handoff .next-command')?.before(summary);
    }
    const lifecycle = activation.lifecycle || {};
    const maintenance = activation.maintenance || {};
    const events = Array.isArray(lifecycle.events) ? lifecycle.events : [];
    document.querySelector('#activation-hooks').textContent = lifecycle.state === 'configured'
      ? `CONFIGURADO · ${events.length || 5} HOOKS`
      : 'NÃO CONFIRMADO';
    document.querySelector('#activation-hook-events').textContent = events.length
      ? events.join(' · ')
      : 'StartSession · UserPromptSubmit · PreToolUse · PostToolUse · Stop';
    const reviewRequired = lifecycle.hook_review === 'owner_review_required';
    document.querySelector('#activation-hook-review').textContent = reviewRequired
      ? 'REVISÃO DO OWNER NECESSÁRIA'
      : String(lifecycle.hook_review || 'NÃO CONFIRMADO').replaceAll('_', ' ').toUpperCase();
    document.querySelector('#activation-hook-review-detail').textContent = reviewRequired
      ? 'Ao abrir o Codex, revise os cinco comandos locais quando ele solicitar. O instalador nunca grava confiança global em seu nome.'
      : 'A revisão de confiança dos hooks segue o estado reportado pelo runtime.';
    document.querySelector('#activation-maintenance').textContent = maintenance.native_observed
      ? 'OBSERVADO NO LAUNCHD'
      : 'NÃO OBSERVADO';
    document.querySelector('#activation-maintenance-detail').textContent = maintenance.native_observed
      ? 'Carregado no login e verificado a cada 15 minutos para recuperar manutenção local pendente.'
      : 'A manutenção agendada ainda não foi carregada pelo macOS.';
    document.querySelector('#activation-native-session').textContent = lifecycle.native_observed === 'unavailable_pending_first_session'
      ? 'AGUARDANDO PRIMEIRA SESSÃO'
      : String(lifecycle.native_observed || 'NÃO OBSERVADO').replaceAll('_', ' ').toUpperCase();
    document.querySelector('#activation-model').textContent = maintenance.model_backed === 'unavailable'
      ? 'INDISPONÍVEL'
      : String(maintenance.model_backed || 'NÃO CONFIGURADO').replaceAll('_', ' ').toUpperCase();
    summary.hidden = false;
  }

  function renderRuntimeTargets(targets = []) {
    const actions = document.querySelector('#runtime-actions');
    const copy = document.querySelector('#runtime-launch-copy');
    if (!actions || !copy) return;
    runtimeTargets = Array.isArray(targets) ? targets : [];
    actions.replaceChildren();
    if (!runtime) {
      copy.textContent = 'No instalador conectado, o Maestro detecta os runtimes disponíveis e abre o workspace no lugar certo.';
    } else if (!runtimeTargets.length) {
      copy.textContent = 'Nenhum runtime compatível foi detectado. Instale Claude Code ou Codex e abra este instalador novamente.';
    } else {
      copy.textContent = 'Claude Code é o caminho principal e abre este workspace no contexto certo. Codex continua disponível como alternativa; cada runtime pede sua própria revisão de hooks.';
      runtimeTargets.forEach((target, index) => {
        const button = document.createElement('button');
        button.type = 'button';
        button.className = index === 0 ? 'primary' : 'quiet runtime-secondary';
        button.dataset.action = 'launch-runtime';
        button.dataset.runtime = target.id;
        button.innerHTML = `<span>${target.label}</span><span class="arrow">↗</span>`;
        actions.append(button);
      });
    }
    const close = document.createElement('button');
    close.type = 'button';
    close.className = 'quiet';
    close.dataset.action = 'close';
    close.textContent = 'Fechar instalador';
    actions.append(close);
  }

  function show(name, { focusHeading = true } = {}) {
    // Errors are scoped to one action. A new scene always starts clean so an
    // empty error container can never survive a successful transition.
    showError('check', '');
    showError('install', '');
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
    // Each installer step occupies the same app scene. Reset scroll roots so
    // navigation never leaves the following step peeking under the current one.
    window.scrollTo({ top: 0, left: 0, behavior: 'instant' });
    document.querySelector('.content')?.scrollTo({ top: 0, left: 0, behavior: 'instant' });
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
      installed = Boolean(state.installed) && !simulation;
      installedCLIPath = state.cli_path || '';
      defaultWorkspace = state.workspace_default || '';
      if (workspaceDefault && defaultWorkspace) workspaceDefault.textContent = defaultWorkspace;
      renderRuntimeTargets(state.runtimes);
      document.body.dataset.mode = 'runtime';
      updateConnectionChrome(state);
      updateModeBanner(state);
      updateFinishCopy();
      runtimePlatform = state.platform || '';
      platformLabel.textContent = state.platform || platform;
      destination.textContent = state.managed_root || destination.textContent;
      if (installed) setFirstCommand(installedCLIPath);
    } catch (_) {
      // Opening index.html directly is a deliberate, non-mutating preview.
    }
  }

  async function verifyRelease() {
    showError('check', '');
    showRuntimeStage('check', 'Conferindo release, assinatura e destino…');
    if (installed) {
      show('finish');
      showStatus('Maestro já está instalado neste perfil. Nenhuma alteração foi necessária.');
      return;
    }
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
    if (installed) {
      show('finish');
      showStatus('Maestro já está instalado neste perfil. Nenhuma alteração foi necessária.');
      return;
    }
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
	  document.body.classList.add('is-installing');
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
	  document.body.classList.remove('is-installing');
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

  async function launchSelectedRuntime(button) {
    showStatus('');
    if (!runtime) {
      showStatus('Esta é uma prévia visual. No instalador conectado, você escolherá um workspace antes de abrir o runtime.');
      return;
    }
    const runtimeID = button?.dataset.runtime;
    if (!runtimeID || !runtimeTargets.some(target => target.id === runtimeID)) {
      showStatus('Esse runtime não está disponível neste computador. Abra o instalador novamente após instalá-lo.');
      return;
    }
    button.disabled = true;
    const label = button.querySelector('span');
    const original = label?.textContent || 'Abrir runtime';
    setButtonLabel(button, 'Escolha seu workspace…');
    try {
      const response = await fetch('/api/launch-runtime', requestOptions('POST', { runtime: runtimeID }));
      const payload = await response.json();
      if (!response.ok) throw new Error(payload.error || 'Não foi possível abrir o runtime.');
      const runtimeName = runtimeID === 'claude' ? 'Claude Code' : 'Codex no ChatGPT';
      showStatus(`${runtimeName} foi iniciado em ${payload.workspace_path}. O Maestro continua disponível no seu perfil.`);
    } catch (error) {
      showStatus(error.message);
    } finally {
      setButtonLabel(button, original);
      button.disabled = false;
    }
  }

  async function createWorkspace(button) {
    showStatus('');
    if (!runtime) {
      showStatus('Esta é uma prévia visual. No instalador conectado, o Maestro cria o workspace padrão antes de abrir um runtime.');
      return;
    }
    const importExisting = button?.dataset.import === 'true';
    const choices = [...document.querySelectorAll('[data-action="create-workspace"]')];
    choices.forEach(choice => { choice.disabled = true; });
    const original = button.querySelector('span')?.textContent || button.textContent;
    setButtonLabel(button, importExisting ? 'Escolha a pasta…' : 'Preparando…');
    setWorkspaceProvisioning('creating');
    try {
      const response = await fetch('/api/create-workspace', requestOptions('POST', { import_existing: importExisting }));
      const payload = await response.json();
      if (!response.ok) throw new Error(payload.error || 'Não foi possível criar seu workspace.');
      defaultWorkspace = payload.workspace_path || defaultWorkspace;
      renderActivation(payload.activation);
      renderRuntimeTargets(runtimeTargets);
      setWorkspaceProvisioning('ready');
      await pause(760);
      showRuntimeHandoff();
      showStatus(payload.source_registered
        ? `Workspace pronto em ${payload.workspace_path}. Claude Code é o caminho principal; a fonte foi registrada para ingestão verificada.`
        : `Workspace pronto em ${payload.workspace_path}. Claude Code está configurado; a primeira sessão ainda precisa observar os hooks.`);
    } catch (error) {
      setWorkspaceProvisioning('');
      showStatus(error.message);
    } finally {
      setButtonLabel(button, original);
      choices.forEach(choice => { choice.disabled = false; });
    }
  }

  async function closeInstaller() {
    if (!runtime) {
      showStatus('Esta é uma prévia visual: feche esta aba quando terminar.');
      window.close();
      return;
    }
    const button = document.querySelector('[data-action="close"]');
    button.disabled = true;
    try {
      const response = await fetch('/api/close', requestOptions('POST'));
      const payload = await response.json();
      if (!response.ok) throw new Error(payload.error || 'Não foi possível encerrar o instalador.');
      showStatus('O instalador foi encerrado com segurança. Esta janela pode ser fechada.');
      window.close();
    } catch (error) {
      showStatus(error.message);
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
    if (next) {
      if (installed) {
        show('finish');
        showStatus('Maestro já está instalado neste perfil. Nenhuma alteração foi necessária.');
      } else show(next.dataset.next);
    }
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
    if (action === 'create-workspace') await createWorkspace(event.target.closest('[data-action="create-workspace"]'));
    if (action === 'open-data') await openDataFolder();
    if (action === 'launch-runtime') await launchSelectedRuntime(event.target.closest('[data-action="launch-runtime"]'));
    if (action === 'copy-path') navigator.clipboard?.writeText(destination.textContent);
    if (action === 'copy-command') await copyFirstCommand();
    if (action === 'close') await closeInstaller();
  });

  renderRuntimeTargets();
  discoverRuntime();
})();
