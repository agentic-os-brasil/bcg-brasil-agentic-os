@echo off
setlocal
set "PSModulePath="
powershell.exe -NoLogo -NoProfile -File "%~dp0Preparar Maestro.ps1"
set "maestro_exit=%ERRORLEVEL%"
pause
exit /b %maestro_exit%
