Unicode true

####
## Please note: Template replacements don't work in this file. They are provided with default defines like
## mentioned underneath.
## If the keyword is not defined, "wails_tools.nsh" will populate them.
## If they are defined here, "wails_tools.nsh" will not touch them. This allows you to use this project.nsi manually
## from outside of Wails for debugging and development of the installer.
## 
## For development first make a wails nsis build to populate the "wails_tools.nsh":
## > wails build --target windows/amd64 --nsis
## Then you can call makensis on this file with specifying the path to your binary:
## For a AMD64 only installer:
## > makensis -DARG_WAILS_AMD64_BINARY=..\..\bin\app.exe
## For a ARM64 only installer:
## > makensis -DARG_WAILS_ARM64_BINARY=..\..\bin\app.exe
## For a installer with both architectures:
## > makensis -DARG_WAILS_AMD64_BINARY=..\..\bin\app-amd64.exe -DARG_WAILS_ARM64_BINARY=..\..\bin\app-arm64.exe
####
## The following information is taken from the wails_tools.nsh file, but they can be overwritten here.
####
## !define INFO_PROJECTNAME    "my-project" # Default "tunnelkit"
## !define INFO_COMPANYNAME    "My Company" # Default "My Company"
## !define INFO_PRODUCTNAME    "My Product Name" # Default "TunnelKit"
## !define INFO_PRODUCTVERSION "1.0.0"     # Default "0.1.0"
## !define INFO_COPYRIGHT      "(c) Now, My Company" # Default "© 2026, My Company"
###
## !define PRODUCT_EXECUTABLE  "Application.exe"      # Default "${INFO_PROJECTNAME}.exe"
## !define UNINST_KEY_NAME     "UninstKeyInRegistry"  # Default "${INFO_COMPANYNAME}${INFO_PRODUCTNAME}"
####
## !define REQUEST_EXECUTION_LEVEL "admin"            # Default "admin"  see also https://nsis.sourceforge.io/Docs/Chapter4.html
## !define WAILS_INSTALL_SCOPE     "user"             # Default "machine" - set to "user" for per-user install ($LOCALAPPDATA) without UAC prompt
####
## Include the wails tools
####
!include "wails_tools.nsh"
!include "LogicLib.nsh"

# The version information for this two must consist of 4 parts
VIProductVersion "${INFO_PRODUCTVERSION}.0"
VIFileVersion    "${INFO_PRODUCTVERSION}.0"

VIAddVersionKey "CompanyName"     "${INFO_COMPANYNAME}"
VIAddVersionKey "FileDescription" "${INFO_PRODUCTNAME} Installer"
VIAddVersionKey "ProductVersion"  "${INFO_PRODUCTVERSION}"
VIAddVersionKey "FileVersion"     "${INFO_PRODUCTVERSION}"
VIAddVersionKey "LegalCopyright"  "${INFO_COPYRIGHT}"
VIAddVersionKey "ProductName"     "${INFO_PRODUCTNAME}"

# Enable HiDPI support. https://nsis.sourceforge.io/Reference/ManifestDPIAware
ManifestDPIAware true

!include "MUI2.nsh"

!define MUI_ICON "..\icon.ico"
!define MUI_UNICON "..\icon.ico"
# !define MUI_WELCOMEFINISHPAGE_BITMAP "resources\leftimage.bmp" #Include this to add a bitmap on the left side of the Welcome Page. Must be a size of 164x314
!define MUI_ABORTWARNING # This will warn the user if they exit from the installer.

!insertmacro MUI_PAGE_WELCOME # Welcome to the installer page.
# !insertmacro MUI_PAGE_LICENSE "resources\eula.txt" # Adds a EULA page to the installer
!insertmacro MUI_PAGE_DIRECTORY # In which folder install page.
!insertmacro MUI_PAGE_INSTFILES # Installing page.

# Launch the desktop client via Windows Explorer so it runs under the signed-in
# user's standard token rather than inheriting the installer's elevated token.
!define MUI_FINISHPAGE_RUN
!define MUI_FINISHPAGE_RUN_FUNCTION "LaunchAsDesktopUser"
!insertmacro MUI_PAGE_FINISH # Finished installation page.

!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES # Uninstalling page

!insertmacro MUI_LANGUAGE "SimpChinese"
!insertmacro MUI_LANGUAGE "English" # Set the Language of the installer

## The following two statements can be used to sign the installer and the uninstaller. The path to the binaries are provided in %1
#!uninstfinalize 'signtool --file "%1"'
#!finalize 'signtool --file "%1"'

Name "${INFO_PRODUCTNAME}"
OutFile "..\..\..\bin\${INFO_PROJECTNAME}-${ARCH}-installer.exe" # Name of the installer's file.

# Use a compile-time sentinel ending in \${INFO_PRODUCTNAME} so NSIS directory
# browsing appends \TunnelKit while .onInit preserves /D= overrides and restores
# the previous 64-bit InstallLocation on upgrades.
!define PLACEHOLDER_INSTALL_DIR "placeholder\${INFO_PRODUCTNAME}"
!if "${WAILS_INSTALL_SCOPE}" == "user"
    !define DEFAULT_INSTALL_DIR "$LOCALAPPDATA\Programs\${INFO_PRODUCTNAME}"
!else
    !define DEFAULT_INSTALL_DIR "$PROGRAMFILES64\${INFO_COMPANYNAME}\${INFO_PRODUCTNAME}"
!endif
InstallDir "${PLACEHOLDER_INSTALL_DIR}"

ShowInstDetails hide
ShowUnInstDetails hide

# Never terminate a privileged VPN helper before it restores host networking.
!macro tunnelkit.requireHelperStopped
    nsExec::Exec '"$SYSDIR\cmd.exe" /d /c tasklist /FI "IMAGENAME eq tunnelkit-helper.exe" /FO CSV /NH | findstr /I "tunnelkit-helper.exe"'
    Pop $0
    ${If} $0 == "0"
        SetDetailsView show
        DetailPrint "检测到正在运行的 VPN 会话（tunnelkit-helper.exe）。"
        MessageBox MB_ICONSTOP "请先断开 VPN 并退出 TunnelKit，再重试安装或卸载。" /SD IDOK
        Abort
    ${EndIf}
!macroend

# Silently close the desktop client only if it is currently running; never emit
# localized taskkill "process not found" errors during fresh installs.
!macro tunnelkit.stopDesktopClient
    nsExec::Exec '"$SYSDIR\taskkill.exe" /F /T /IM "${PRODUCT_EXECUTABLE}"'
    Pop $0
    ${If} $0 == "0"
        DetailPrint "正在关闭运行中的 ${INFO_PRODUCTNAME}..."
        Sleep 500
    ${EndIf}
!macroend

# Run a privileged service management command silently; only pop captured output
# when process creation succeeded ($0 != "error" && $0 != "timeout") and expand
# diagnostic details only on failure.
!macro tunnelkit.execServiceCmd CMD ERR_MSG
    StrCpy $1 ""
    nsExec::ExecToStack `${CMD}`
    Pop $0
    ${If} $0 != "error"
    ${AndIf} $0 != "timeout"
        Pop $1
    ${EndIf}
    ${If} $0 != "0"
        SetDetailsView show
        ${If} $1 != ""
            DetailPrint "$1"
        ${EndIf}
        MessageBox MB_ICONSTOP "${ERR_MSG}" /SD IDOK
        Abort
    ${EndIf}
!macroend

!macro tunnelkit.writeInstallMetadata
    SetRegView 64
    !if "${WAILS_INSTALL_SCOPE}" == "user"
        WriteRegStr HKCU "${UNINST_KEY}" "InstallLocation" "$INSTDIR"
        WriteRegDWORD HKCU "${UNINST_KEY}" "NoModify" 1
        WriteRegDWORD HKCU "${UNINST_KEY}" "NoRepair" 1
    !else
        WriteRegStr HKLM "${UNINST_KEY}" "InstallLocation" "$INSTDIR"
        WriteRegDWORD HKLM "${UNINST_KEY}" "NoModify" 1
        WriteRegDWORD HKLM "${UNINST_KEY}" "NoRepair" 1
    !endif
!macroend

Function LaunchAsDesktopUser
    SetOutPath "$INSTDIR"
    !if "${REQUEST_EXECUTION_LEVEL}" == "admin"
        Exec '"$WINDIR\explorer.exe" "$INSTDIR\${PRODUCT_EXECUTABLE}"'
    !else
        Exec '"$INSTDIR\${PRODUCT_EXECUTABLE}"'
    !endif
FunctionEnd

Function .onInit
    !insertmacro wails.checkArchitecture

    ${If} $INSTDIR == "${PLACEHOLDER_INSTALL_DIR}"
        SetRegView 64
        !if "${WAILS_INSTALL_SCOPE}" == "user"
            ReadRegStr $0 HKCU "${UNINST_KEY}" "InstallLocation"
        !else
            ReadRegStr $0 HKLM "${UNINST_KEY}" "InstallLocation"
        !endif
        ${If} $0 != ""
            StrCpy $INSTDIR "$0"
        ${Else}
            StrCpy $INSTDIR "${DEFAULT_INSTALL_DIR}"
        ${EndIf}
    ${EndIf}
FunctionEnd

Section
    !insertmacro wails.setShellContext

    DetailPrint "正在检查运行环境..."
    !insertmacro tunnelkit.requireHelperStopped
    !insertmacro tunnelkit.stopDesktopClient

    !insertmacro wails.webview2runtime

    DetailPrint "正在部署 ${INFO_PRODUCTNAME} 程序文件..."
    SetOutPath "$INSTDIR"
    Delete "$INSTDIR\*.old"

    !insertmacro wails.files
    !ifdef ARG_WAILS_HELPER_BINARY
        File "/oname=tunnelkit-helper.exe" "${ARG_WAILS_HELPER_BINARY}"
    !else if /FileExists "..\..\..\bin\tunnelkit-helper-windows-${ARCH}.exe"
        File "/oname=tunnelkit-helper.exe" "..\..\..\bin\tunnelkit-helper-windows-${ARCH}.exe"
    !else
        File "..\..\..\bin\tunnelkit-helper.exe"
    !endif
    !if /FileExists "..\..\..\bin\wintun-${ARCH}.dll"
        File "/oname=wintun.dll" "..\..\..\bin\wintun-${ARCH}.dll"
    !else if /FileExists "..\..\..\bin\wintun.dll"
        File "..\..\..\bin\wintun.dll"
    !endif

    DetailPrint "正在配置 ${INFO_PRODUCTNAME} 系统辅助服务..."
    !insertmacro tunnelkit.execServiceCmd '"$INSTDIR\tunnelkit-helper.exe" install-service' "无法安装 TunnelKit 特权辅助服务，请确认已授予管理员权限。"

    CreateShortcut "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"
    CreateShortCut "$DESKTOP\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"

    !insertmacro wails.associateFiles
    !insertmacro wails.associateCustomProtocols

    !insertmacro wails.writeUninstaller
    !insertmacro tunnelkit.writeInstallMetadata
SectionEnd

Section "uninstall"
    !insertmacro wails.setShellContext

    DetailPrint "正在检查运行环境..."
    !insertmacro tunnelkit.requireHelperStopped
    !insertmacro tunnelkit.stopDesktopClient

    ${If} ${FileExists} "$INSTDIR\tunnelkit-helper.exe"
        DetailPrint "正在移除 ${INFO_PRODUCTNAME} 系统辅助服务..."
        !insertmacro tunnelkit.execServiceCmd '"$INSTDIR\tunnelkit-helper.exe" uninstall-service' "无法停止并卸载 TunnelKit 特权辅助服务，请先断开 VPN 后重试。"
    ${Else}
        nsExec::Exec '"$SYSDIR\sc.exe" stop TunnelKitHelper'
        Pop $0
        nsExec::Exec '"$SYSDIR\sc.exe" delete TunnelKitHelper'
        Pop $0
        RMDir /r "$PROGRAMFILES64\TunnelKit Helper"
    ${EndIf}

    SetShellVarContext current
    RMDir /r "$AppData\${PRODUCT_EXECUTABLE}" # Remove the WebView2 DataPath
    !insertmacro wails.setShellContext

    Delete "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk"
    Delete "$DESKTOP\${INFO_PRODUCTNAME}.lnk"

    !insertmacro wails.unassociateFiles
    !insertmacro wails.unassociateCustomProtocols

    SetOutPath "$TEMP"
    !insertmacro wails.deleteUninstaller
    RMDir /r "$INSTDIR"
    !if "${WAILS_INSTALL_SCOPE}" != "user"
        RMDir "$PROGRAMFILES64\${INFO_COMPANYNAME}"
        ClearErrors
    !endif
SectionEnd
