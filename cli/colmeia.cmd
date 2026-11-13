@echo off
setlocal
rem Node.js no PATH: usa. Senao, usa o Colmeia.exe como Node (COLMEIA_NODE).
rem Sem bloco ( ): errorlevel dentro de parenteses expande antes do node rodar.
where node >nul 2>nul
if not errorlevel 1 goto :usenode
if defined COLMEIA_NODE goto :useelectron
echo colmeia: Node.js nao encontrado. Abra o terminal pelo app Colmeia ou instale https://nodejs.org 1>&2
exit /b 9009
:usenode
node "%~dp0colmeia.mjs" %*
exit /b %errorlevel%
:useelectron
set ELECTRON_RUN_AS_NODE=1
"%COLMEIA_NODE%" "%~dp0colmeia.mjs" %*
exit /b %errorlevel%
