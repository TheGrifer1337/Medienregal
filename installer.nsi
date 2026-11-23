Unicode true
!include "MUI2.nsh"
!include "FileFunc.nsh"
Name "Medatheke by Maximilian"
OutFile "${OUTFILE}"
InstallDir "$LOCALAPPDATA\Programs\Medienregal"
InstallDirRegKey HKCU "Software\Medienregal" "InstallPath"
RequestExecutionLevel user
SetCompressor /SOLID lzma
VIProductVersion "${VERSION}.0.0"
VIAddVersionKey "ProductName" "Medatheke by Maximilian"
VIAddVersionKey "FileDescription" "Medatheke by Maximilian – Windows-Installer"
VIAddVersionKey "FileVersion" "${VERSION}"
VIAddVersionKey "ProductVersion" "${VERSION}"
VIAddVersionKey "CompanyName" "Maximilian"
VIAddVersionKey "LegalCopyright" "Medienregal"

!define MUI_ICON "winres/bookshelf.ico"
!define MUI_UNICON "winres/bookshelf.ico"
!define MUI_ABORTWARNING
!define WEBVIEWKEY "Software\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}"
!insertmacro MUI_PAGE_WELCOME
!define MUI_PAGE_CUSTOMFUNCTION_LEAVE VerifyDestination
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!define MUI_FINISHPAGE_RUN "$INSTDIR\Medienregal.exe"
!define MUI_FINISHPAGE_RUN_TEXT "Medatheke öffnen"
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "German"

Var Updating
Var RetryCount

Function .onInit
  ${GetParameters} $0
  ClearErrors
  ${GetOptions} $0 "/UPDATE" $1
  IfErrors done
  StrCpy $Updating "1"
  ReadRegStr $1 HKCU "Software\Medienregal" "InstallPath"
  StrCmp $1 $INSTDIR 0 invalid
  IfFileExists "$INSTDIR\Medienregal-Deinstallieren.exe" done invalid
  invalid:
    SetErrorLevel 2
    Quit
  done:
FunctionEnd

Function VerifyDestination
  IfFileExists "$INSTDIR\Medienregal.exe" 0 done
  ReadRegStr $0 HKCU "Software\Medienregal" "InstallPath"
  StrCmp $0 $INSTDIR done
  MessageBox MB_ICONEXCLAMATION "Wähle einen Ordner ohne bereits vorhandene Medienregal.exe."
  Abort
  done:
FunctionEnd

Function DetectWebView
  ReadRegStr $0 HKCU "${WEBVIEWKEY}" "pv"
  StrCmp $0 "" localMissing
  StrCmp $0 "0.0.0.0" localMissing detected
  localMissing:
    ReadRegStr $0 HKLM "${WEBVIEWKEY}" "pv"
    StrCmp $0 "0.0.0.0" 0 detected
    StrCpy $0 ""
  detected:
FunctionEnd

Section "Medatheke by Maximilian"
  SetShellVarContext current
  Call DetectWebView
  StrCmp $0 "" 0 runtimeReady
  InitPluginsDir
  SetOutPath "$PLUGINSDIR"
  File "/oname=WebView2Bootstrapper.exe" "${BOOTSTRAPPER}"
  ExecWait '"$PLUGINSDIR\WebView2Bootstrapper.exe" /silent /install' $1
  Call DetectWebView
  StrCmp $0 "" 0 runtimeReady
  IfSilent +2
    MessageBox MB_ICONSTOP "Microsoft WebView2 konnte nicht installiert werden. Prüfe deine Internetverbindung und starte den Installer erneut."
  SetErrorLevel 3
  Abort
  runtimeReady:
  SetOutPath "$INSTDIR"
  ClearErrors
  File "/oname=Medienregal.neu" "${APPEXE}"
  IfErrors failed
  IfFileExists "$INSTDIR\Medienregal.exe" 0 installNew
  ReadRegStr $0 HKCU "Software\Medienregal" "InstallPath"
  StrCmp $0 $INSTDIR 0 failed
  StrCpy $RetryCount "0"
  waitForApp:
    ClearErrors
    Delete "$INSTDIR\Medienregal.alt"
    ClearErrors
    Rename "$INSTDIR\Medienregal.exe" "$INSTDIR\Medienregal.alt"
    IfErrors 0 installNew
    StrCmp $Updating "1" 0 failed
    IntOp $RetryCount $RetryCount + 1
    IntCmp $RetryCount 40 failed 0 failed
    Sleep 250
    Goto waitForApp
  installNew:
    ClearErrors
    Rename "$INSTDIR\Medienregal.neu" "$INSTDIR\Medienregal.exe"
    IfErrors restore
    ClearErrors
    WriteUninstaller "$INSTDIR\Medienregal-Deinstallieren.exe"
    IfErrors restoreAfterSwitch
    Delete "$INSTDIR\Medienregal.alt"
    WriteRegStr HKCU "Software\Medienregal" "InstallPath" "$INSTDIR"
    WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\Medienregal" "DisplayName" "Medatheke by Maximilian"
    WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\Medienregal" "DisplayVersion" "${VERSION}"
    WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\Medienregal" "DisplayIcon" "$INSTDIR\Medienregal.exe"
    WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\Medienregal" "Publisher" "Maximilian"
    WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\Medienregal" "InstallLocation" "$INSTDIR"
    WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\Medienregal" "UninstallString" '"$INSTDIR\Medienregal-Deinstallieren.exe"'
    WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\Medienregal" "NoModify" 1
    WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\Medienregal" "NoRepair" 1
    Delete "$SMPROGRAMS\Medienregal.lnk"
    CreateShortcut "$SMPROGRAMS\Medatheke by Maximilian.lnk" "$INSTDIR\Medienregal.exe"
    Goto done
  restoreAfterSwitch:
    Delete "$INSTDIR\Medienregal.exe"
  restore:
    IfFileExists "$INSTDIR\Medienregal.alt" 0 failed
    Rename "$INSTDIR\Medienregal.alt" "$INSTDIR\Medienregal.exe"
  failed:
    Delete "$INSTDIR\Medienregal.neu"
    IfSilent +2
      MessageBox MB_ICONSTOP "Die Installation ist fehlgeschlagen. Schließe Medienregal und prüfe, ob der gewählte Ordner beschreibbar ist."
    SetErrorLevel 1
    Abort
  done:
SectionEnd

Section "Uninstall"
  SetShellVarContext current
  ClearErrors
  Delete "$INSTDIR\Medienregal.exe"
  IfErrors busy
  Delete "$INSTDIR\Medienregal.alt"
  Delete "$INSTDIR\Medienregal.neu"
  Delete "$SMPROGRAMS\Medienregal.lnk"
  Delete "$SMPROGRAMS\Medatheke by Maximilian.lnk"
  DeleteRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\Medienregal"
  DeleteRegKey HKCU "Software\Medienregal"
  Delete "$INSTDIR\Medienregal-Deinstallieren.exe"
  RMDir "$INSTDIR"
  Goto done
  busy:
    IfSilent +2
      MessageBox MB_ICONEXCLAMATION "Schließe Medienregal und starte die Deinstallation erneut."
    SetErrorLevel 1
    Abort
  done:
SectionEnd
