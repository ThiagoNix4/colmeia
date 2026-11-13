; Instalador Windows (electron-builder NSIS).
; O colmeiad roda destacado e sobrevive ao fechar o app; ao atualizar, para o daemon antigo
; para que o novo binário seja usado (os cards do workspace são recriados ao abrir o app).

; Para o daemon: --stop gracioso de cada colmeiad.exe staged, depois taskkill /F de reserva.
!macro colmeiaStopDaemon
  Push $0
  Push $1
  FindFirst $0 $1 "$PROFILE\.local\share\colmeia\*"
  colmeiaLoop:
    StrCmp $1 "" colmeiaDone
    IfFileExists "$PROFILE\.local\share\colmeia\$1\colmeiad.exe" 0 colmeiNext
      nsExec::Exec '"$PROFILE\.local\share\colmeia\$1\colmeiad.exe" --stop'
    colmeiNext:
    FindNext $0 $1
    Goto colmeiaLoop
  colmeiaDone:
  FindClose $0
  Sleep 1500
  nsExec::Exec 'taskkill /F /IM colmeiad.exe'
  Pop $1
  Pop $0
!macroend

!macro customInit
  !insertmacro colmeiaStopDaemon
!macroend

!macro customUnInstall
  !insertmacro colmeiaStopDaemon
  RMDir /r "$PROFILE\.local\share\colmeia"
!macroend
