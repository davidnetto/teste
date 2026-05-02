@echo off
REM run.bat — atalho para subir o servidor Florentina via PowerShell.
REM Duplo-clique para executar.

cd /d "%~dp0"
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0run.ps1"
pause
