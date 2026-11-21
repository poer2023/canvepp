!insertmacro MUI_LANGUAGE "SimpChinese"
LangString MYGO_DESKTOP_SHORTCUT ${LANG_SIMPCHINESE} "创建桌面快捷方式"

!define CANVEPP_LICENSES "${__FILEDIR__}/../assets"

!define CANVEPP_RUNTIME "${__FILEDIR__}/../build/runtime/win32-x64"
!define CANVEPP_CODEX_LICENSES "${__FILEDIR__}/licenses"

!macro customInstall
  SetOutPath "$INSTDIR\Licenses"
  File "/oname=Lucide.txt" "${CANVEPP_LICENSES}/lucide-LICENSE"
  File "/oname=go-pinyin.txt" "${CANVEPP_LICENSES}/go-pinyin-LICENSE"
  File "${CANVEPP_CODEX_LICENSES}/Codex-LICENSE.txt"
  File "${CANVEPP_CODEX_LICENSES}/Codex-NOTICE.txt"
  File "${CANVEPP_CODEX_LICENSES}/go-x-image-LICENSE.txt"
  SetOutPath "$INSTDIR\Licenses\documents"
  File "${CANVEPP_CODEX_LICENSES}/documents/*.txt"
  SetOutPath "$INSTDIR\runtime"
  File /r "${CANVEPP_RUNTIME}/*"
  SetOutPath "$INSTDIR"
!macroend

!macro customUnInstall
  DeleteRegKey HKCU "Software\Classes\canvepp-result"
  DeleteRegKey HKCU "Software\Classes\AppUserModelId\app.canvepp.native"
!macroend
