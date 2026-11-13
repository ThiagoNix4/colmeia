@echo off
setlocal
rem Node.js no PATH: usa. Senao, usa o Colmeia.exe como Node (COLMEIA_NODE, definido pelo app).
where node >nul 2>nul
if %errorlevel%==0 (
  node "%~dp0colmeia.mjs" %*
  exit /b %errorlevel%
)
if defined COLMEIA_NODE (
  set ELECTRON_RUN_AS_NODE=1
  "%COLMEIA_NODE%" "%~dp0colmeia.mjs" %*
  exit /b %errorlevel%
)
echo colmeia: Node.js nao encontrado. Abra o terminal pelo app Colmeia ou instale https://nodejs.org 1>&2
exit /b 9009
