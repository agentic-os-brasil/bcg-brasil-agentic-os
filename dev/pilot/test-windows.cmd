@echo off
powershell.exe -NoLogo -NoProfile -File "%~dp0Testar Maestro.ps1"
set "maestro_exit=%ERRORLEVEL%"
if not "%maestro_exit%"=="0" pause
exit /b %maestro_exit%
