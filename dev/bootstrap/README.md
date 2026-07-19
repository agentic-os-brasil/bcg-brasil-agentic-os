# Contributor bootstrap

These scripts prepare a source contributor after the private repository has been cloned. They are development infrastructure and never enter `bcgos` or an OS bundle.

## Windows

From the repository root in PowerShell:

```powershell
& .\dev\bootstrap\windows.ps1
```

The script checks Git and Go, configures repository-local Git identity when needed, installs repository-owned hooks, runs the initial validation and leaves one next action. It never installs software, changes credentials, requests administrator access or discards files.

For CI smoke tests only:

```powershell
& .\dev\bootstrap\windows.ps1 -NonInteractive
```

The pre-clone and dependency-installation conversation is defined in `docs/onboarding/windows-contributor-prompt.md`.
