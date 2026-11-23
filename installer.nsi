Unicode true
!include "MUI2.nsh"
!include "FileFunc.nsh"
Name "Medienregal"
OutFile "${OUTFILE}"
InstallDir "$LOCALAPPDATA\Programs\Medienregal"
InstallDirRegKey HKCU "Software\Medienregal" "InstallPath"
RequestExecutionLevel user
SetCompressor /SOLID lzma
VIProductVersion "${VERSION}.0.0"
VIAddVersionKey "ProductName" "Medienregal"
VIAddVersionKey "FileDescription" "Medienregal – Windows-Installer"
VIAddVersionKey "FileVersion" "${VERSION}"
VIAddVersionKey "ProductVersion" "${VERSION}"
VIAddVersionKey "CompanyName" "Medienregal"
VIAddVersionKey "LegalCopyright" "Medienregal"

!define MUI_ABORTWARNING
!insertmacro MUI_PAGE_WELCOME
!define MUI_PAGE_CUSTOMFUNCTION_LEAVE VerifyDestination
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!define MUI_FINISHPAGE_RUN "$INSTDIR\Medienregal.exe"
!define MUI_FINISHPAGE_RUN_TEXT "Medienregal öffnen"
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

Section "Medienregal"
  SetShellVarContext current
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
    WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\Medienregal" "DisplayName" "Medienregal"
    WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\Medienregal" "DisplayVersion" "${VERSION}"
    WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\Medienregal" "DisplayIcon" "$INSTDIR\Medienregal.exe"
    WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\Medienregal" "Publisher" "Medienregal"
    WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\Medienregal" "InstallLocation" "$INSTDIR"
    WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\Medienregal" "UninstallString" '"$INSTDIR\Medienregal-Deinstallieren.exe"'
    WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\Medienregal" "NoModify" 1
    WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\Medienregal" "NoRepair" 1
    CreateShortcut "$SMPROGRAMS\Medienregal.lnk" "$INSTDIR\Medienregal.exe"
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
