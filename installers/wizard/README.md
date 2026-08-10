# Maestro visual installer

The Windows Canary Simple wizard is a dependency-free visual shell with one
linear path: choose **Nova instalação** or **Atualizar**, watch the automatic
progress, then continue in Claude Desktop in the prepared Maestro workspace.

The UI is intentionally a thin client. When connected it uses the installer
bridge for the transaction and launch; it does not describe optional runtime
integrations as installed. A direct `index.html` preview is non-mutating.

Only a terminal transaction failure is shown to the user, in plain language,
with **Tentar novamente**. The UI feature-detects a future `/api/simple-install`
endpoint and falls back to the existing bridge endpoints while that endpoint is
being introduced.
