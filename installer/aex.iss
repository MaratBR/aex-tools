; Inno Setup script for the Windows installer (built by bin\dist.ps1; see BUILD.md).
; Installs for the current user only (no admin): aex.exe, aex-cli.exe, plugins, LICENSE and NOTICE into
; %LOCALAPPDATA%\Programs\aex, with aex in the Start menu. The data folder (%APPDATA%\aex) is kept on
; uninstall.
;
; Defines passed by dist.ps1 (iscc /D...): Version, Arch (amd64 or arm64), Source (the folder holding the
; built files), OutDir, OutName.

#ifndef Version
  #error Pass /DVersion=<version>
#endif
#ifndef Arch
  #define Arch "amd64"
#endif
#ifndef Source
  #error Pass /DSource=<folder with aex.exe>
#endif
#ifndef OutDir
  #define OutDir "..\dist"
#endif
#ifndef OutName
  #define OutName "aex-" + Version + "-windows-" + Arch + "-setup"
#endif

#if Arch == "arm64"
  #define Archs "arm64"
#else
  #define Archs "x64compatible"
#endif

[Setup]
AppId={{C06DC533-F981-44E1-A26A-245FB07953D8}
AppName=aex tools
AppVersion={#Version}
AppVerName=aex tools {#Version}
AppPublisher=Marat B
AppPublisherURL=https://github.com/MaratBR/aex-tools
AppCopyright=Copyright (c) 2026 Marat B
PrivilegesRequired=lowest
DefaultDirName={userpf}\aex
DisableProgramGroupPage=yes
DisableDirPage=auto
ArchitecturesAllowed={#Archs}
ArchitecturesInstallIn64BitMode={#Archs}
OutputDir={#OutDir}
OutputBaseFilename={#OutName}
SetupIconFile=..\assets\logo.ico
UninstallDisplayIcon={app}\aex.exe
UninstallDisplayName=aex tools
WizardStyle=modern
Compression=lzma2
SolidCompression=yes
; Closes a running aex (the window or a plugin) before replacing its files.
CloseApplications=yes

[Tasks]
Name: desktopicon; Description: "Create a &desktop shortcut"; Flags: unchecked

[Files]
Source: "{#Source}\aex.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#Source}\aex-cli.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#Source}\plugins\*.exe"; DestDir: "{app}\plugins"; Flags: ignoreversion
Source: "{#Source}\LICENSE"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#Source}\NOTICE"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
; The same aex.lnk aex itself adds to the Start menu (internal/shortcut), so it does not offer to add it.
Name: "{userprograms}\aex"; Filename: "{app}\aex.exe"; WorkingDir: "{%USERPROFILE}"; Comment: "aex work tools"
Name: "{userdesktop}\aex"; Filename: "{app}\aex.exe"; WorkingDir: "{%USERPROFILE}"; Comment: "aex work tools"; Tasks: desktopicon

[Run]
Filename: "{app}\aex.exe"; Description: "Open aex"; Flags: nowait postinstall skipifsilent

[UninstallDelete]
; Old exes a build or an update renamed aside while they were running.
Type: files; Name: "{app}\*.old*"
Type: files; Name: "{app}\plugins\*.old*"

[Code]
// aex starts on login through an "aex" value in the Run key (internal/autostart); remove it with aex.
procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
begin
  if CurUninstallStep = usUninstall then
    RegDeleteValue(HKEY_CURRENT_USER, 'Software\Microsoft\Windows\CurrentVersion\Run', 'aex');
end;
