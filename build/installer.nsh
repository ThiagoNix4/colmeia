; Instalador Windows (electron-builder NSIS).
; O colmeiad roda destacado e sobrevive ao fechar o app; ao atualizar, para o daemon antigo
; para que o novo binário seja usado (os cards do workspace são recriados ao abrir o app).
!macro customInit
  nsExec::Exec 'taskkill /F /IM colmeiad.exe'
!macroend
