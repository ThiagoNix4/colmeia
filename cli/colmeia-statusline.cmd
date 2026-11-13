@echo off
setlocal
rem Statusline do Claude Code (Windows): node do PATH ou, sem Node, o Colmeia.exe (COLMEIA_NODE).
where node >nul 2>nul
if %errorlevel%==0 (
  node "%~dp0colmeia-statusline.mjs"
  exit /b 0
)
if defined COLMEIA_NODE (
  set ELECTRON_RUN_AS_NODE=1
  "%COLMEIA_NODE%" "%~dp0colmeia-statusline.mjs"
)
exit /b 0
