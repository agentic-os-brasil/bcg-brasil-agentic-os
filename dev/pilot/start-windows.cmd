@echo off
"%SystemRoot%\System32\WindowsPowerShell\v1.0\powershell.exe" -NoProfile -File "%~dp0Preparar Maestro.ps1"
set "maestro_result=%errorlevel%"
if not "%maestro_result%"=="0" echo O preparo foi recusado. Preserve a mensagem e os arquivos; nao altere a politica do computador.
pause
exit /b %maestro_result%
