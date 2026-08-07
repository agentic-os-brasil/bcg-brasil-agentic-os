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
  const intentMessage = document.querySelector('#intent-message');
  const intentSubcopy = document.querySelector('#intent-subcopy');
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
  let installedVersion = '';
  let workspaceFlowSelection = null;
  let workspaceFlowAnalysis = null;
  let workspaceFlowReceipt = null;
  let workspaceFlowScene = null;
  let verificationRun = 0;
  let selectedIntent = 'fresh';
  const platform = /Win/i.test(navigator.userAgent) ? 'Windows' : /Mac/i.test(navigator.userAgent) ? 'macOS' : 'seu dispositivo';
  const reducedMotion = window.matchMedia?.('(prefers-reduced-motion: reduce)').matches;

  platformLabel.textContent = platform;
  destination.textContent = platform === 'Windows' ? '%LOCALAPPDATA%\\BCGOS' : platform === 'macOS' ? '~/Library/Application Support/BCGOS' : '~/.local/share/bcgos';

  const checkNames = ['release', 'signature', 'space'];

  function setCheckState(name, state, label) {
    const item = document.querySelector(`.check-item[data-check="${name}"]`);
    if (!item) return;
    const icon = item.querySelector('.check-icon');
    const status = item.querySelector('strong');
    item.classList.remove('is-checking', 'is-ready', 'is-simulated', 'is-error');
    item.classList.add(`is-${state}`);
    const copy = {
      idle: ['◌', 'aguardando'],
      checking: ['◌', 'analisando'],
      ready: ['✓', 'confirmado'],
      simulated: ['◇', 'ensaio'],
      error: ['×', 'não concluído'],
    }[state] || ['◌', 'aguardando'];
    icon.textContent = copy[0];
    status.textContent = label || copy[1];
    status.setAttribute('aria-label', `${item.querySelector('b')?.textContent || name}: ${status.textContent}`);
  }

  function resetChecks() {
    checkNames.forEach(name => setCheckState(name, 'idle'));
  }

  function markChecks(status) {
    const state = status === 'ready' ? 'ready' : status === 'simulated' ? 'simulated' : status === 'error' ? 'error' : 'idle';
    checkNames.forEach(name => setCheckState(name, state));
  }

  async function animateCheckProgress(run) {
    resetChecks();
    // The API is atomic, but the UI gives each contract a visible handoff so
    // the owner can follow the verification before the next scene opens.
    const labels = {
      release: 'Conferindo release autorizado',
      signature: 'Validando assinatura e integridade',
      space: 'Confirmando espaço de usuário',
    };
    for (const [index, name] of checkNames.entries()) {
      if (run !== verificationRun) return;
      setCheckState(name, 'checking');
      setProgressBar('verification', 12 + index * 24, labels[name]);
      await pause(340);
    }
    setProgressBar('verification', 84, 'Aguardando confirmação do core');
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
    return new Promise(resolve => window.setTimeout(resolve, reducedMotion ? 0 : milliseconds));
  }

  function setIntent(intent) {
    selectedIntent = intent || 'fresh';
    const copy = {
      fresh: ['Vou começar agora — estou muito empolgado. 🎼', 'Escolha uma direção; o Maestro conduzirá a próxima etapa sem tocar nos seus arquivos.'],
      import: ['Já tenho um workspace/second brain para o Maestro ingerir. 🧠', 'Primeiro criamos o workspace do Maestro; depois você autoriza uma fonte para análise local bounded.'],
      update: ['Estou atualizando meu Maestro — vou direcionar onde continuar. 🔄', 'A atualização preserva seu workspace e só avança depois de explicar o plano.'],
    }[selectedIntent] || ['Vou começar agora — estou muito empolgado. 🎼', 'Escolha uma direção; o Maestro conduzirá a próxima etapa sem tocar nos seus arquivos.'];
    if (intentMessage) intentMessage.textContent = copy[0];
    if (intentSubcopy) intentSubcopy.textContent = copy[1];
    document.querySelectorAll('[data-intent]').forEach(button => {
      const selected = button.dataset.intent === selectedIntent;
      button.classList.toggle('is-selected', selected);
      button.setAttribute('aria-pressed', String(selected));
    });
  }

  function selectIntent(intent) {
    setIntent(intent);
    if (installed) {
      show('finish');
      return;
    }
    show('check');
    document.querySelector('[data-action="verify"]')?.focus({ preventScroll: true });
  }

  function setProgressBar(kind, value, label) {
    const prefix = kind === 'install' ? 'installation' : 'verification';
    const bar = document.querySelector(`#${prefix}-progress-bar`);
    const track = bar?.closest('[role="progressbar"]');
    const valueNode = document.querySelector(`#${prefix}-progress-value`);
    const labelNode = document.querySelector(`#${prefix}-progress-label`);
    const bounded = Math.max(0, Math.min(100, Math.round(value)));
    if (bar) bar.style.width = `${bounded}%`;
    if (track) track.setAttribute('aria-valuenow', String(bounded));
    if (valueNode) valueNode.textContent = `${bounded}%`;
    if (labelNode && label) labelNode.textContent = label;
  }

  function resetProgressBars() {
    setProgressBar('verification', 0, 'Pronto para conferir o release');
    setProgressBar('install', 0, 'Aguardando sua confirmação');
  }

  function runOnboardingDemo() {
    const demo = document.querySelector('.onboarding-demo');
    if (!demo) return;
    demo.classList.remove('is-playing');
    // Restart the timeline intentionally so the user can revisit the story.
    void demo.offsetWidth;
    demo.classList.add('is-playing');
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
    const flow = document.querySelector('#workspace-flow');
    if (flow) flow.hidden = true;
    const handoff = document.querySelector('#runtime-handoff');
    handoff.hidden = false;
    updateFinishCopy();
    alignActiveScene(handoff);
    window.requestAnimationFrame(() => {
      handoff.scrollTop = 0;
      handoff.querySelector('h1')?.focus({ preventScroll: true });
    });
  }

  function escapeHTML(value) {
    return String(value || '').replace(/[&<>'"]/g, character => ({
      '&': '&amp;', '<': '&lt;', '>': '&gt;', "'": '&#39;', '"': '&quot;'
    }[character]));
  }

  function flowModeLabel(mode) {
    return {
      update: 'Atualizar o Maestro',
      workspace_migration: 'Migrar um workspace Maestro',
      external_import: 'Importar uma pasta externa'
    }[mode] || 'Continuar';
  }

  function flowClassificationLabel(classification) {
    return {
      maestro_update: 'Atualização do Maestro',
      maestro_workspace: 'Workspace Maestro',
      external_folder: 'Pasta externa'
    }[classification] || classification || 'Fonte analisada';
  }

  function flowListMarkup(title, items, emptyCopy) {
    const rows = Array.isArray(items) && items.length
      ? items.map(item => `<li><code>${escapeHTML(item.path)}</code><span>${escapeHTML(item.reason)}</span></li>`).join('')
      : `<li class="is-empty"><span>${escapeHTML(emptyCopy)}</span></li>`;
    return `<article class="flow-classification"><h3>${escapeHTML(title)}</h3><ul>${rows}</ul></article>`;
  }

  function isValidWorkspaceReceipt(receipt, expectedStatus) {
    if (!receipt?.valid || !receipt.ready || receipt.status !== expectedStatus || !receipt.receipt_id || !receipt.operation) return false;
    const required = { staging: 'completed', validation: 'completed', rollback: 'available' };
    const seen = new Set();
    for (const stage of Array.isArray(receipt.stages) ? receipt.stages : []) {
      if (!(stage.id in required) || seen.has(stage.id) || stage.status !== required[stage.id] || !String(stage.detail || '').trim()) continue;
      seen.add(stage.id);
    }
    return Object.keys(required).every(id => seen.has(id));
  }

  function isValidWorkspaceFlowReceipt(receipt, expectedStatus = 'committed') {
    if (!isValidWorkspaceReceipt(receipt, expectedStatus)) return false;
    return receipt.source_effect === 'preserved'
      && (receipt.operation !== 'external_import' || receipt.target_effect === 'bounded_import_committed')
      && (receipt.operation !== 'external_import' || receipt.rollback_effect === 'available_from_import_receipt')
      && String(receipt.target_effect || '').trim()
      && String(receipt.rollback_effect || '').trim()
      && receipt.approval_action === 'IMPORT'
      && receipt.approved_by
      && receipt.approval_plan_id === receipt.plan_id
      && receipt.plan_id
      && receipt.plan_digest;
  }

  function isValidWorkspaceFlowRollback(receipt) {
    return Boolean(receipt?.valid && !receipt.ready && receipt.status === 'rolled_back' && receipt.receipt_id
      && receipt.source_effect === 'preserved' && receipt.target_effect === 'bounded_import_rolled_back' && receipt.rollback_effect === 'completed'
      && receipt.approval_action === 'ROLLBACK' && receipt.plan_id && receipt.plan_digest);
  }

  function createWorkspaceFlowScene() {
    if (workspaceFlowScene) return workspaceFlowScene;
    const legacySetup = document.querySelector('#workspace-setup');
    if (legacySetup) legacySetup.hidden = true;
    workspaceFlowScene = document.createElement('div');
    workspaceFlowScene.id = 'workspace-flow';
    workspaceFlowScene.className = 'workspace-flow';
    workspaceFlowScene.innerHTML = `
      <div id="workspace-flow-choice" class="workspace-flow-choice" aria-labelledby="workspace-flow-title">
        <span class="eyebrow">ETAPA 04 · SEU WORKSPACE</span>
        <h1 id="workspace-flow-title" tabindex="-1">Seu caminho está definido.<br><em>Agora vamos preparar.</em></h1>
        <p class="lead" id="workspace-flow-lead">A escolha feita no início orienta esta etapa. O Maestro só pede uma nova decisão quando ela realmente muda o efeito sobre seus dados.</p>
        <div id="intent-primary-slot" class="intent-primary-slot"></div>
        <div class="flow-options" id="advanced-flow-options" aria-label="Outras jornadas disponíveis">
          <button class="flow-option" type="button" data-action="workspace-flow" data-flow-mode="update" data-requires-installed="true">
            <span class="flow-option-number">01</span><span><b>Atualizar o Maestro</b><small id="flow-update-copy">Preserva o workspace e mostra a migração de versão necessária.</small><em id="flow-update-version"></em></span><span class="arrow">↗</span>
          </button>
          <button class="flow-option" type="button" data-action="workspace-flow" data-flow-mode="workspace_migration">
            <span class="flow-option-number">02</span><span><b>Migrar um workspace Maestro</b><small>Escolha um workspace existente para analisar identidade, manifesto e projeção.</small></span><span class="arrow">↗</span>
          </button>
          <button class="flow-option" type="button" data-action="workspace-flow" data-flow-mode="external_import">
            <span class="flow-option-number">03</span><span><b>Importar uma pasta externa</b><small>A pasta vira uma fonte analisada; não é tratada como workspace e não é ingerida só por ser selecionada.</small></span><span class="arrow">↗</span>
          </button>
          <button class="flow-option flow-option-secondary" type="button" data-action="create-clean-workspace">
            <span class="flow-option-number">04</span><span><b>Começar com um workspace novo</b><small>Cria o workspace padrão local e só mostra pronto quando o receipt de readiness for válido.</small></span><span class="arrow">↗</span>
          </button>
        </div>
        <button class="quiet advanced-flow-toggle" id="advanced-flow-toggle" type="button" data-action="show-advanced-flow">Ver outras jornadas</button>
        <div class="flow-boundary"><span>REGRA DE SEGURANÇA</span><p>Selecionar uma pasta apenas cria um ponteiro temporário. A análise local bounded lê metadados e calcula o plano; nada é copiado ou mutado até IMPORT.</p></div>
        <div class="callout runtime-error" id="workspace-flow-error" role="alert" hidden><span>!</span><p></p></div>
        <div class="runtime-statusline" id="workspace-flow-feedback" role="status" aria-live="polite" hidden><span class="pulse"></span><p></p></div>
      </div>
      <div id="workspace-flow-analysis" class="workspace-flow-analysis" hidden aria-labelledby="workspace-flow-analysis-title"></div>
      <div id="workspace-flow-result" class="workspace-flow-result" hidden aria-labelledby="workspace-flow-result-title"></div>`;
    document.querySelector('#runtime-handoff')?.before(workspaceFlowScene);
    renderWorkspaceFlowChoices();
    return workspaceFlowScene;
  }

  function renderWorkspaceFlowChoices() {
    const scene = createWorkspaceFlowScene();
    if (!scene) return;
    const slot = scene.querySelector('#intent-primary-slot');
    const advanced = scene.querySelector('#advanced-flow-options');
    const toggle = scene.querySelector('#advanced-flow-toggle');
    const lead = scene.querySelector('#workspace-flow-lead');
    const intentCopy = {
      fresh: ['Criar meu workspace Maestro', 'Workspace novo e local; nada de outras pastas será lido nesta etapa.', false],
      import: ['Criar workspace e escolher fonte', 'Seu workspace vem primeiro. Depois você escolhe a pasta que será registrada para análise bounded.', true],
      update: ['Verificar atualização', 'O Maestro confere a versão instalada e mostra o plano antes de alterar qualquer coisa.', false],
    }[selectedIntent] || ['Criar meu workspace Maestro', 'Workspace novo e local; nada de outras pastas será lido nesta etapa.', false];
    if (slot) {
      slot.innerHTML = `<button class="primary intent-primary-action" type="button" data-action="${selectedIntent === 'update' && installed ? 'workspace-flow' : 'create-workspace'}" ${selectedIntent === 'update' && installed ? 'data-flow-mode="update"' : `data-import="${intentCopy[2]}"`}><span>${intentCopy[0]}</span><span class="arrow">↗</span></button><small>${intentCopy[1]}</small>`;
    }
    if (lead) lead.textContent = selectedIntent === 'import'
      ? 'Você já sinalizou que tem uma fonte de memória. O Maestro cria o workspace primeiro e só depois abre a escolha autorizada.'
      : selectedIntent === 'update'
      ? 'Você escolheu atualizar. O Maestro mantém o workspace e mostra o plano de mudança antes de agir.'
      : 'Você escolheu começar agora. O Maestro cria o workspace local e só mostra pronto depois do receipt de readiness.';
    if (advanced) advanced.hidden = true;
    if (toggle) toggle.hidden = false;
    const update = scene.querySelector('[data-flow-mode="update"]');
    if (update) {
      update.hidden = !installed;
      update.disabled = !runtime || !installed;
    }
    scene.querySelectorAll('[data-flow-mode]:not([data-flow-mode="update"])').forEach(button => {
      button.disabled = !runtime;
    });
    const clean = scene.querySelector('[data-action="create-clean-workspace"]');
    if (clean) clean.disabled = !runtime;
    const version = scene.querySelector('#flow-update-version');
    if (version) version.textContent = installedVersion ? `Versão atual detectada: ${installedVersion}` : 'A versão atual será lida pelo core transacional conectado.';
  }

  function showAdvancedWorkspaceFlow() {
    const scene = createWorkspaceFlowScene();
    const advanced = scene?.querySelector('#advanced-flow-options');
    const slot = scene?.querySelector('#intent-primary-slot');
    const toggle = scene?.querySelector('#advanced-flow-toggle');
    if (advanced) advanced.hidden = false;
    if (slot) slot.replaceChildren();
    if (toggle) toggle.hidden = true;
  }

  function setWorkspaceFlowFeedback(message, isError = false) {
    const error = document.querySelector('#workspace-flow-error');
    const feedback = document.querySelector('#workspace-flow-feedback');
    if (error) {
      error.hidden = !isError || !message;
      error.querySelector('p').textContent = isError ? message || '' : '';
    }
    if (feedback) {
      feedback.hidden = isError || !message;
      feedback.querySelector('p').textContent = !isError ? message || '' : '';
    }
  }

  function setWorkspaceFlowPhase(phase) {
    document.querySelectorAll('#workspace-flow-analysis [data-flow-phase]').forEach(step => {
      const active = step.dataset.flowPhase === phase;
      step.classList.toggle('is-active', active);
      step.classList.toggle('is-done', ['analysis', 'plan', 'confirm', 'staging', 'validation', 'rollback'].indexOf(step.dataset.flowPhase) < ['analysis', 'plan', 'confirm', 'staging', 'validation', 'rollback'].indexOf(phase));
    });
  }

  function renderWorkspaceFlowAnalysis(analysis) {
    workspaceFlowAnalysis = analysis;
    const choice = document.querySelector('#workspace-flow-choice');
    const target = document.querySelector('#workspace-flow-analysis');
    if (!target) return;
    if (choice) choice.hidden = true;
    target.hidden = false;
    const blocked = analysis.state === 'blocked';
    const capabilities = (analysis.capabilities_unavailable || []).map(capability => `<li><b>${escapeHTML(capability.id)} · ${escapeHTML(capability.state)}</b><span>${escapeHTML(capability.message)}</span></li>`).join('');
    const blockers = (analysis.blockers || []).map(blocker => `<li><b>${escapeHTML(blocker.code)}</b><span>${escapeHTML(blocker.message)}</span></li>`).join('');
    const effects = `<div class="flow-effects" aria-label="Efeitos do plano"><article><span>FONTE</span><b>${escapeHTML(analysis.source_effect || 'não informado')}</b></article><article><span>ALVO</span><b>${escapeHTML(analysis.target_effect || 'não informado')}</b></article><article><span>ROLLBACK</span><b>${escapeHTML(analysis.rollback_effect || 'não informado')}</b></article></div>`;
    target.innerHTML = `
      <span class="eyebrow">${blocked ? 'ANÁLISE BLOQUEADA' : 'ANÁLISE CONCLUÍDA'} · ${escapeHTML(flowClassificationLabel(analysis.classification))}</span>
      <h1 id="workspace-flow-analysis-title" tabindex="-1">${blocked ? 'Não é possível prosseguir.' : 'Este é o plano.'}<br><em>${blocked ? 'O bloqueio fica explícito.' : 'Você decide se ele segue.'}</em></h1>
      <p class="lead">${escapeHTML(analysis.summary)}</p>
      <div class="flow-analysis-meta"><span>FONTE</span><b>${escapeHTML(analysis.source?.label || 'Fonte selecionada')}</b><small>${escapeHTML(analysis.source_effect === 'preserved' ? 'A origem permanece preservada; a análise é somente leitura.' : 'Efeito na origem não autorizado.')}</small></div>
      ${effects}
      ${analysis.migration_required ? `<div class="flow-version-callout"><b>${escapeHTML(analysis.installed_version || 'versão atual')}</b><span>→</span><b>${escapeHTML(analysis.target_version || 'versão alvo')}</b><small>${escapeHTML(analysis.migration_summary || 'Migração necessária antes de concluir.')}</small></div>` : ''}
      <div class="flow-classifications">${flowListMarkup('Mapeados', analysis.mapped, 'Nenhum item mapeado nesta análise.')}${flowListMarkup('Excluídos', analysis.excluded, 'Nenhum item excluído nesta análise.')}${flowListMarkup('Ambíguos', analysis.ambiguous, 'Nenhum item ambíguo nesta análise.')}</div>
      <article class="flow-capability"><h3>Capability unavailable</h3><p>O que ainda não está disponível fica explícito e não vira uma promessa de ingestão.</p><ul>${capabilities || '<li><span>Nenhuma capability indisponível foi reportada.</span></li>'}</ul></article>
      ${blocked ? `<article class="flow-blockers" role="alert"><h3>Bloqueios</h3><ul>${blockers || '<li><span>O core não autorizou esta jornada.</span></li>'}</ul></article>` : ''}
      <div class="flow-confirm-boundary"><span>${analysis.plan_digest ? `PLANO · ${escapeHTML(analysis.plan_digest)}` : 'PLANO INDISPONÍVEL'}</span><p>${blocked ? 'Nenhum staging, confirmação ou instalação ocorrerá enquanto houver bloqueios.' : 'O plano será confirmado antes do staging. A origem permanece preservada e o rollback continua disponível.'}</p></div>
      <div class="panel-actions"><button class="quiet" type="button" data-action="workspace-flow-back">Escolher outra fonte</button>${analysis.can_confirm ? '<button class="primary" type="button" data-action="confirm-workspace-flow"><span>Confirmar e preparar</span><span class="arrow">↗</span></button>' : '<button class="primary" type="button" disabled aria-disabled="true"><span>Confirmação indisponível</span></button>'}</div>
      <div class="callout runtime-error" id="workspace-flow-analysis-error" role="alert" hidden><span>!</span><p></p></div>`;
    setWorkspaceFlowPhase('plan');
    const heading = target.querySelector('h1');
    if (heading) window.requestAnimationFrame(() => heading.focus({ preventScroll: true }));
  }

  function renderWorkspaceFlowProgress(mode, errorMessage = '') {
    const target = document.querySelector('#workspace-flow-analysis');
    if (!target) return;
    const title = mode === 'confirm' ? 'Preparando com rastreabilidade.' : 'Analisando a fonte sem tocar nela.';
    const failure = errorMessage
      ? `<div class="callout runtime-error" role="alert"><span>!</span><p>${escapeHTML(errorMessage)}</p></div><div class="panel-actions"><button class="quiet" type="button" data-action="workspace-flow-back">Escolher outra fonte</button></div>`
      : '';
    target.hidden = false;
    target.innerHTML = `<span class="eyebrow">MAESTRO EM MOVIMENTO</span><h1 id="workspace-flow-analysis-title" tabindex="-1">${title}<br><em>cada estado fica visível.</em></h1><div class="flow-phase-track" role="list" aria-label="Estado da jornada"><div data-flow-phase="analysis" role="listitem"><b>01</b><span>Análise</span><small>classificar a fonte</small></div><div data-flow-phase="plan" role="listitem"><b>02</b><span>Plano</span><small>explicar o escopo</small></div><div data-flow-phase="confirm" role="listitem"><b>03</b><span>Confirmação</span><small>uma decisão explícita</small></div><div data-flow-phase="staging" role="listitem"><b>04</b><span>Staging</span><small>preparar sem substituir</small></div><div data-flow-phase="validation" role="listitem"><b>05</b><span>Validação</span><small>conferir o resultado</small></div><div data-flow-phase="rollback" role="listitem"><b>06</b><span>Rollback</span><small>manter a volta disponível</small></div></div><div class="flow-progress-note${errorMessage ? ' is-error' : ''}" role="status" aria-live="polite">${escapeHTML(errorMessage || (mode === 'confirm' ? 'Aguardando o receipt válido do core transacional.' : 'A análise local bounded está lendo metadados e preparando o plano.'))}</div>${failure}`;
    setWorkspaceFlowPhase(mode === 'confirm' ? 'staging' : 'analysis');
    const heading = target.querySelector('h1');
    if (heading) window.requestAnimationFrame(() => heading.focus({ preventScroll: true }));
  }

  function renderWorkspaceFlowReceipt(receipt) {
    const target = document.querySelector('#workspace-flow-result');
    if (!target) return;
    document.querySelector('#workspace-flow-analysis').hidden = true;
    target.hidden = false;
    const stages = (receipt.stages || []).map(stage => `<li><b>${escapeHTML(stage.id)}</b><span>${escapeHTML(stage.status)}</span><small>${escapeHTML(stage.detail)}</small></li>`).join('');
    const rolledBack = receipt.status === 'rolled_back';
    const resultCopy = rolledBack
      ? 'O rollback foi concluído pelo core e o alvo voltou ao estado anterior deste receipt.'
      : receipt.operation === 'external_import'
      ? 'A importação bounded foi concluída no alvo. Conversão e ingestão de documentos continuam separadas e itens não disponíveis permanecem excluídos ou em quarentena.'
      : receipt.operation === 'workspace_migration'
      ? 'A migração do workspace foi confirmada pelo core autorizado e a origem permanece preservada.'
      : receipt.operation === 'new_workspace'
      ? `O workspace ${escapeHTML(receipt.workspace_path || 'local')} foi preparado e validado pelo installer.`
      : 'A atualização do Maestro foi confirmada; o workspace continua fora da transação.';
    target.innerHTML = `<span class="eyebrow">RECEIPT VÁLIDO · ${escapeHTML(receipt.receipt_id)}</span><h1 id="workspace-flow-result-title" tabindex="-1">${rolledBack ? 'Rollback concluído.' : 'Pronto para o próximo passo.'}<br><em>${rolledBack ? 'o alvo foi revertido.' : 'com a volta preservada.'}</em></h1><p class="lead">${resultCopy}</p><div class="flow-receipt-card"><span>RECEIPT</span><code>${escapeHTML(receipt.receipt_id)}</code><strong>STATUS · ${escapeHTML(receipt.status)}</strong><ul>${stages}</ul></div><div class="flow-effects" aria-label="Efeitos confirmados"><article><span>FONTE</span><b>${escapeHTML(receipt.source_effect)}</b></article><article><span>ALVO</span><b>${escapeHTML(receipt.target_effect)}</b></article><article><span>ROLLBACK</span><b>${escapeHTML(receipt.rollback_effect)}</b></article></div><div class="flow-boundary"><span>${rolledBack ? 'ROLLBACK CONFIRMADO' : 'FONTE PRESERVADA'}</span><p>${rolledBack ? 'Nenhum novo import pode reutilizar a confirmação consumida.' : 'O wizard só mostra pronto porque o receipt está válido, vinculado ao plano e confirma staging, validação e rollback.'}</p></div><div class="panel-actions">${!rolledBack && receipt.operation === 'external_import' ? '<button class="quiet" type="button" data-action="rollback-workspace-flow">Reverter este import</button>' : ''}<button class="quiet" type="button" data-action="close">Fechar instalador</button></div>`;
    const heading = target.querySelector('h1');
    if (heading) window.requestAnimationFrame(() => heading.focus({ preventScroll: true }));
  }

  async function startWorkspaceFlow(mode) {
    createWorkspaceFlowScene();
    setWorkspaceFlowFeedback('');
    if (!runtime) {
      setWorkspaceFlowFeedback('Esta é uma prévia visual: o modo conectado selecionará a fonte e fará a análise local bounded sem mutação.', true);
      return;
    }
    const choices = document.querySelectorAll('[data-flow-mode]');
    choices.forEach(choice => { choice.disabled = true; });
    renderWorkspaceFlowProgress('analysis');
    setWorkspaceFlowPhase('analysis');
    try {
      const selectionResponse = await fetch('/api/workspace-flow/select', requestOptions('POST', { mode }));
      const selection = await selectionResponse.json();
      if (!selectionResponse.ok) throw new Error(selection.error || 'Não foi possível selecionar esta fonte.');
      workspaceFlowSelection = selection;
      await pause(320);
      const analysisResponse = await fetch('/api/workspace-flow/analyze', requestOptions('POST', { flow_id: selection.flow_id }));
      const analysis = await analysisResponse.json();
      renderWorkspaceFlowAnalysis(analysis);
      if (!analysisResponse.ok && analysis.state !== 'blocked') throw new Error(analysis.error || 'A análise não pôde ser concluída.');
    } catch (error) {
      const target = document.querySelector('#workspace-flow-analysis');
      if (target) target.hidden = true;
      document.querySelector('#workspace-flow-choice').hidden = false;
      setWorkspaceFlowFeedback(error.message, true);
    } finally {
      choices.forEach(choice => { choice.disabled = !runtime || (choice.dataset.requiresInstalled === 'true' && !installed); });
    }
  }

  async function confirmWorkspaceFlow() {
    if (!workspaceFlowSelection || !workspaceFlowAnalysis?.plan_digest) return;
    const button = document.querySelector('[data-action="confirm-workspace-flow"]');
    if (button) button.disabled = true;
    renderWorkspaceFlowProgress('confirm');
    try {
      const response = await fetch('/api/workspace-flow/confirm', requestOptions('POST', { flow_id: workspaceFlowSelection.flow_id, plan_digest: workspaceFlowAnalysis.plan_digest, action: 'IMPORT' }));
      const receipt = await response.json();
      if (!response.ok) throw new Error(receipt.error || 'A confirmação foi interrompida com segurança.');
      if (!isValidWorkspaceFlowReceipt(receipt, 'committed')) throw new Error('O core não retornou um receipt de import válido; o wizard não pode mostrar pronto.');
      workspaceFlowReceipt = receipt;
      renderWorkspaceFlowReceipt(receipt);
      setWorkspaceFlowFeedback('');
    } catch (error) {
      renderWorkspaceFlowProgress('confirm', error.message);
    } finally {
      if (button) button.disabled = false;
    }
  }

  async function rollbackWorkspaceFlow() {
    const receipt = workspaceFlowReceipt;
    if (!workspaceFlowSelection || !receipt?.receipt_id || !workspaceFlowAnalysis?.plan_digest) return;
    renderWorkspaceFlowProgress('confirm');
    try {
      const response = await fetch('/api/workspace-flow/rollback', requestOptions('POST', { flow_id: workspaceFlowSelection.flow_id, plan_digest: workspaceFlowAnalysis.plan_digest, receipt_id: receipt.receipt_id, action: 'ROLLBACK' }));
      const rolledBack = await response.json();
      if (!response.ok) throw new Error(rolledBack.error || 'O rollback foi bloqueado com segurança.');
      if (!isValidWorkspaceFlowRollback(rolledBack)) throw new Error('O core não retornou um receipt de rollback válido.');
      workspaceFlowReceipt = rolledBack;
      renderWorkspaceFlowReceipt(rolledBack);
    } catch (error) {
      renderWorkspaceFlowProgress('confirm', error.message);
    }
  }

  async function createCleanWorkspace() {
    createWorkspaceFlowScene();
    if (!runtime) {
      setWorkspaceFlowFeedback('Esta é uma prévia visual: o modo conectado criará o workspace e emitirá o receipt de readiness.', true);
      return;
    }
    renderWorkspaceFlowProgress('confirm');
    try {
      const response = await fetch('/api/create-workspace', requestOptions('POST', { import_existing: false }));
      const payload = await response.json();
      if (!response.ok) throw new Error(payload.error || 'Não foi possível criar o workspace.');
      const receipt = payload.receipt;
      if (!isValidWorkspaceReceipt(receipt, 'ready')) throw new Error('O core não retornou um receipt de readiness válido; o wizard não pode mostrar pronto.');
      renderWorkspaceFlowReceipt(receipt);
    } catch (error) {
      renderWorkspaceFlowProgress('confirm', error.message);
    }
  }

  function resetWorkspaceFlow() {
    workspaceFlowSelection = null;
    workspaceFlowAnalysis = null;
    workspaceFlowReceipt = null;
    const scene = createWorkspaceFlowScene();
    scene.querySelector('#workspace-flow-choice').hidden = false;
    scene.querySelector('#workspace-flow-analysis').hidden = true;
    scene.querySelector('#workspace-flow-result').hidden = true;
    setWorkspaceFlowFeedback('');
    renderWorkspaceFlowChoices();
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
    runtimeTargets = (Array.isArray(targets) ? targets : [])
      .slice()
      .sort((a, b) => Number(b.id === 'claude') - Number(a.id === 'claude'));
    actions.replaceChildren();
    if (!runtime) {
      copy.textContent = 'No instalador conectado, o Maestro detecta os runtimes disponíveis e abre o workspace no lugar certo.';
    } else if (!runtimeTargets.length) {
      copy.textContent = 'Nenhum runtime compatível foi detectado. Instale Claude Code ou Codex e abra este instalador novamente.';
    } else {
      copy.textContent = 'Seu workspace está pronto. Abra o Claude Code para começar.';
      runtimeTargets.forEach((target, index) => {
        const button = document.createElement('button');
        button.type = 'button';
        button.className = index === 0 ? 'primary' : 'quiet runtime-secondary';
        button.dataset.action = 'launch-runtime';
        button.dataset.runtime = target.id;
        const label = target.id === 'claude' ? 'Abrir no Claude Code Desktop' : target.label;
        button.innerHTML = `<span>${label}</span><span class="arrow">↗</span>`;
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

  function alignActiveScene(panel) {
    if (!panel) return;
    panel.dataset.fitsViewport = 'false';
    panel.scrollTop = 0;
    window.requestAnimationFrame(() => {
      // The panel, rather than the document, owns overflow on desktop. First
      // reset that real scroll root, then center only scenes that actually fit.
      panel.scrollTop = 0;
      panel.dataset.fitsViewport = panel.scrollHeight <= panel.clientHeight + 1 ? 'true' : 'false';
    });
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
      finish: 'Etapa 4 de 4: seu workspace. Siga o caminho escolhido e aguarde um receipt válido.'
    };
    if (stageAnnouncement) stageAnnouncement.textContent = announcements[name] || '';
    // Each installer step occupies the same app scene. Reset scroll roots so
    // navigation never leaves the following step peeking under the current one.
    window.scrollTo({ top: 0, left: 0, behavior: 'instant' });
    const activePanel = document.querySelector(`[data-panel="${name}"]`);
    alignActiveScene(activePanel);
    if (focusHeading) {
      const heading = activePanel?.querySelector('h1');
      if (heading) window.requestAnimationFrame(() => heading.focus({ preventScroll: true }));
    }
    if (name === 'check') {
      setProgress('verify');
      if (!verified) setProgressBar('verification', 0, 'Pronto para conferir o release');
    }
    if (name === 'install') setProgress('install');
    if (name === 'finish') setProgress('complete');
    if (name === 'finish') {
      const flow = createWorkspaceFlowScene();
      flow.hidden = false;
      document.querySelector('#runtime-handoff').hidden = true;
      renderWorkspaceFlowChoices();
    }
    if (name === 'check' && !verified) {
      verificationRun += 1;
      resetChecks();
      if (mode === 'preview' && !runtime) {
        const run = verificationRun;
        window.setTimeout(async () => {
          if (runtime) return;
          await animateCheckProgress(run);
          if (run === verificationRun && !runtime) markChecks('simulated');
        }, 180);
      }
    }
    if (name === 'finish' && !document.querySelector('#workspace-setup')?.hidden) {
      window.setTimeout(runOnboardingDemo, 180);
    }
  }

  async function discoverRuntime() {
    updateConnectionChrome();
    try {
      const response = await fetch('/api/state');
      if (!response.ok) return;
      const state = await response.json();
      const wasRuntime = runtime;
      runtime = true;
      simulation = state.mode === 'simulation';
      installed = Boolean(state.installed) && !simulation;
      installedCLIPath = state.cli_path || '';
      installedVersion = state.installed_version || '';
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
      if (!wasRuntime && !verified) {
        verificationRun += 1;
        resetChecks();
      }
      if (installed) setFirstCommand(installedCLIPath);
      renderWorkspaceFlowChoices();
    } catch (_) {
      // Opening index.html directly is a deliberate, non-mutating preview.
    }
  }

  async function verifyRelease() {
    showError('check', '');
    showRuntimeStage('check', 'Conferindo release, assinatura e destino…');
    if (installed) {
      setProgressBar('verification', 100, 'Maestro já está instalado neste perfil');
      show('finish');
      showStatus('Maestro já está instalado neste perfil. Nenhuma alteração foi necessária.');
      return;
    }
    if (!runtime) {
      setProgressBar('verification', 100, 'Prévia visual concluída');
      show('install');
      showRuntimeStage('check', '');
      return;
    }
    const button = document.querySelector('[data-action="verify"]');
    button.disabled = true;
    setButtonLabel(button, 'Conferindo…');
    setProgressBar('verification', 8, 'Iniciando verificação local');
    try {
      const run = ++verificationRun;
      const progress = animateCheckProgress(run);
      const response = await fetch('/api/verify', requestOptions('POST'));
      const payload = await response.json();
      if (!response.ok) throw new Error(payload.error || 'Não foi possível verificar este release.');
      await progress;
      markChecks(simulation ? 'simulated' : 'ready');
      setProgressBar('verification', 100, simulation ? 'Ensaio técnico conferido' : 'Release conferido com sucesso');
      verified = true;
      planDigest = payload.plan_digest || '';
      setProgress('install');
      show('install');
    } catch (error) {
      verificationRun += 1;
      markChecks('error');
      setProgressBar('verification', 0, 'Verificação interrompida — veja o motivo abaixo');
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
      setProgressBar('install', 100, 'Maestro já está instalado neste perfil');
      show('finish');
      showStatus('Maestro já está instalado neste perfil. Nenhuma alteração foi necessária.');
      return;
    }
    if (!runtime) {
      setProgressBar('install', 100, 'Prévia visual concluída');
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
	setProgressBar('install', 12, 'Preparando o destino no seu perfil');
	  document.body.classList.add('is-installing');
    try {
	  setProgressBar('install', 38, 'Ativando o core em uma transação atômica');
      const response = await fetch('/api/install', requestOptions('POST', { plan_digest: planDigest }));
      const payload = await response.json();
      if (!response.ok) throw new Error(payload.error || 'A instalação foi interrompida com segurança.');
	  setProgressBar('install', 86, 'Conferindo receipt da instalação');
      setProgress('complete');
      setProgressBar('install', 100, simulation ? 'Ensaio concluído com segurança' : 'Instalação concluída no seu perfil');
      show('finish');
      if (simulation) setSimulationCommandHint();
      else setFirstCommand(payload.cli_path);
      showStatus(simulation
        ? `Ensaio concluído. Sandbox de dados: ${payload.data_root}`
        : `Instalação concluída. Dados do usuário: ${payload.data_root}`);
    } catch (error) {
      setProgressBar('install', 0, 'Instalação interrompida — nada foi marcado como pronto');
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
      if (!isValidWorkspaceReceipt(payload.receipt, 'ready')) throw new Error('O core não retornou um receipt de readiness válido; o wizard não pode mostrar pronto.');
      defaultWorkspace = payload.workspace_path || defaultWorkspace;
      renderActivation(payload.activation);
      renderRuntimeTargets(runtimeTargets);
      setWorkspaceProvisioning('ready');
      await pause(760);
      showRuntimeHandoff();
      showStatus(payload.source_registered
        ? `Workspace pronto em ${payload.workspace_path}. A fonte foi registrada apenas como ponteiro; nenhuma ingestão ocorreu.`
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
    const intent = event.target.closest('[data-intent]')?.dataset.intent;
    if (intent) {
      selectIntent(intent);
      return;
    }
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
    if (action === 'workspace-flow') await startWorkspaceFlow(event.target.closest('[data-flow-mode]')?.dataset.flowMode);
    if (action === 'confirm-workspace-flow') await confirmWorkspaceFlow();
    if (action === 'rollback-workspace-flow') await rollbackWorkspaceFlow();
    if (action === 'workspace-flow-back') resetWorkspaceFlow();
    if (action === 'show-advanced-flow') showAdvancedWorkspaceFlow();
    if (action === 'create-clean-workspace') await createCleanWorkspace();
    if (action === 'create-workspace') await createWorkspace(event.target.closest('[data-action="create-workspace"]'));
    if (action === 'replay-demo') runOnboardingDemo();
    if (action === 'open-data') await openDataFolder();
    if (action === 'launch-runtime') await launchSelectedRuntime(event.target.closest('[data-action="launch-runtime"]'));
    if (action === 'copy-path') navigator.clipboard?.writeText(destination.textContent);
    if (action === 'copy-command') await copyFirstCommand();
    if (action === 'close') await closeInstaller();
  });

  renderRuntimeTargets();
  resetProgressBars();
  setIntent('fresh');
  discoverRuntime();
  alignActiveScene(document.querySelector('[data-panel].is-visible'));
})();
