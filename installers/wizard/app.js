(() => {
  const panels = [...document.querySelectorAll('[data-panel]')];
  const steps = [...document.querySelectorAll('.step')];
  const platformLabel = document.querySelector('#platform-label');
  const destination = document.querySelector('#install-destination');
  const mode = document.body.dataset.mode || 'preview';
  let runtime = false;
  let verified = false;
  const platform = /Win/i.test(navigator.userAgent) ? 'Windows' : /Mac/i.test(navigator.userAgent) ? 'macOS' : 'seu dispositivo';

  platformLabel.textContent = platform;
  destination.textContent = platform === 'Windows' ? '%LOCALAPPDATA%\\BCGOS' : platform === 'macOS' ? '~/Library/Application Support/BCGOS' : '~/.local/share/bcgos';

  function markChecks(status) {
    document.querySelectorAll('.check-item').forEach(item => {
      item.querySelector('.check-icon').textContent = status === 'ready' ? '✓' : '◌';
      item.querySelector('.check-icon').style.color = status === 'ready' ? 'var(--teal)' : '';
      item.querySelector('strong').textContent = status === 'ready' ? 'pronto' : 'aguardando';
      item.querySelector('strong').style.color = status === 'ready' ? 'var(--teal)' : '';
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

  function updateFinishCopy() {
    const lead = document.querySelector('#finish-lead');
    if (!lead) return;
    lead.textContent = runtime
      ? 'O Maestro foi instalado no seu perfil. Abra a pasta dele e escolha um workspace de teste quando estiver pronto.'
      : 'Esta é uma prévia visual. Nenhum arquivo foi instalado; no modo real, o Maestro ficará no seu perfil.';
  }

  function show(name) {
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
    });
    if (name === 'check' && mode === 'preview' && !runtime) {
      window.setTimeout(() => {
        document.querySelectorAll('.check-item').forEach((item, index) => {
          window.setTimeout(() => {
            item.querySelector('.check-icon').textContent = '✓';
            item.querySelector('.check-icon').style.color = 'var(--teal)';
            item.querySelector('strong').textContent = 'pronto';
            item.querySelector('strong').style.color = 'var(--teal)';
          }, index * 320);
        });
      }, 180);
    }
  }

  async function discoverRuntime() {
    try {
      const response = await fetch('/api/state');
      if (!response.ok) return;
      const state = await response.json();
      runtime = true;
      document.body.dataset.mode = 'runtime';
      updateFinishCopy();
      platformLabel.textContent = state.platform || platform;
      destination.textContent = state.managed_root || destination.textContent;
      document.querySelector('.footer b').textContent = 'instalação conectada';
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
      const response = await fetch('/api/verify', { method: 'POST' });
      const payload = await response.json();
      if (!response.ok) throw new Error(payload.error || 'Não foi possível verificar este release.');
      markChecks('ready');
      verified = true;
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
      const response = await fetch('/api/install', { method: 'POST' });
      const payload = await response.json();
      if (!response.ok) throw new Error(payload.error || 'A instalação foi interrompida com segurança.');
      show('finish');
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
      const response = await fetch('/api/open-data', { method: 'POST' });
      const payload = await response.json();
      if (!response.ok) throw new Error(payload.error || 'Não foi possível abrir a pasta do Maestro.');
      showStatus(`Pasta aberta: ${payload.path}`);
    } catch (error) {
      showStatus(error.message);
    } finally {
      button.disabled = false;
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
      document.querySelector('[data-next="check"]').focus();
    }
    if (action === 'verify') await verifyRelease();
    if (action === 'install') await installRelease();
    if (action === 'open-data') await openDataFolder();
    if (action === 'copy-path') navigator.clipboard?.writeText(destination.textContent);
    if (action === 'copy-command') navigator.clipboard?.writeText('bcgos doctor');
    if (action === 'close') window.close();
  });

  discoverRuntime();
})();
