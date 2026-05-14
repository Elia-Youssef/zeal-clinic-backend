; Inno Setup installer for Zeal Clinic.
;
; Build the binary first (writes to build\local\output\ZealClinic.exe):
;     go build -trimpath -ldflags "-s -w" -o build\local\output\ZealClinic.exe .\cmd\server
;
; Then compile the installer (produces build\local\output\ZealClinicSetup-<ver>.exe):
;     "C:\Program Files (x86)\Inno Setup 6\ISCC.exe" build\local\installer.iss

#define AppName       "Zeal Clinic"
; Version is read from the repo-root VERSION file, the single source of truth
; shared with the Makefile (ldflags) so the installer and binary stay in sync.
#define VerHandle     FileOpen("..\..\VERSION")
#define AppVersion    Trim(FileRead(VerHandle))
#expr                 FileClose(VerHandle)
#define AppPublisher  "Zeal Clinic"
#define AppURL        ""
#define AppExeName    "ZealClinic.exe"
#define AppPort       "55555"

[Setup]
AppId={{99045ACA-9054-42EB-A841-883400FBF09A}
AppName={#AppName}
AppVersion={#AppVersion}
AppVerName={#AppName} {#AppVersion}
AppPublisher={#AppPublisher}
AppPublisherURL={#AppURL}
AppSupportURL={#AppURL}
AppUpdatesURL={#AppURL}
DefaultDirName={autopf}\{#AppName}
DefaultGroupName={#AppName}
UninstallDisplayName={#AppName}
UninstallDisplayIcon={app}\icon.ico
SetupIconFile=icon.ico
OutputDir=output
OutputBaseFilename=ZealClinicSetup-{#AppVersion}
Compression=lzma2
SolidCompression=yes
WizardStyle=modern
PrivilegesRequired=admin
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
; Detect and shut down any running Zeal Clinic before overwriting files on upgrade.
CloseApplications=yes
RestartApplications=no
; Hide the Select-Start-Menu-Folder page (we always use {#AppName}).
DisableProgramGroupPage=yes

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"

[Tasks]
Name: "desktopicon"; Description: "{cm:CreateDesktopIcon}"; GroupDescription: "{cm:AdditionalIcons}"; Flags: unchecked
Name: "startupicon"; Description: "Start {#AppName} automatically when Windows starts"; GroupDescription: "{cm:AdditionalIcons}"

[Files]
Source: "output\ZealClinic.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "icon.ico"; DestDir: "{app}"; Flags: ignoreversion
; Ship .env next to the exe. Overwritten on upgrade (same as the exe).
Source: ".env"; DestDir: "{app}"; Flags: ignoreversion uninsneveruninstall

[Dirs]
; Shared data directory (holds clinic.db). Give standard users modify rights so
; a non-admin launch of ZealClinic.exe can read/write the database.
Name: "{commonappdata}\{#AppName}"; Permissions: users-modify

[Icons]
Name: "{group}\{#AppName}"; Filename: "{app}\{#AppExeName}"; IconFilename: "{app}\icon.ico"
Name: "{group}\{cm:UninstallProgram,{#AppName}}"; Filename: "{uninstallexe}"
Name: "{autodesktop}\{#AppName}"; Filename: "{app}\{#AppExeName}"; IconFilename: "{app}\icon.ico"; Tasks: desktopicon
Name: "{userstartup}\{#AppName}"; Filename: "{app}\{#AppExeName}"; Parameters: "--startup"; IconFilename: "{app}\icon.ico"; Tasks: startupicon

[Run]
; Firewall: delete any stale rule from a previous install, then add the current one.
; The delete will fail silently on a fresh install (no rule yet), which is fine.
Filename: "netsh"; Parameters: "advfirewall firewall delete rule name=""{#AppName}"""; Flags: runhidden
Filename: "netsh"; Parameters: "advfirewall firewall add rule name=""{#AppName}"" dir=in action=allow protocol=TCP localport={#AppPort} program=""{app}\{#AppExeName}"" description=""{#AppName} backend (HTTP)"""; Flags: runhidden; StatusMsg: "Configuring Windows Firewall..."
; Initialise the database (creates %PROGRAMDATA%\Zeal Clinic\clinic.db if absent).
Filename: "{app}\{#AppExeName}"; Parameters: "--seed-only --no-browser"; Flags: runhidden waituntilterminated; StatusMsg: "Initialising database..."
; Final-page checkbox: launch the app.
Filename: "{app}\{#AppExeName}"; Description: "Launch {#AppName} now"; Flags: nowait postinstall skipifsilent

[UninstallRun]
Filename: "netsh"; Parameters: "advfirewall firewall delete rule name=""{#AppName}"""; Flags: runhidden; RunOnceId: "DelFirewallRule"

[Code]
procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
var
  DataDir: String;
begin
  if CurUninstallStep = usPostUninstall then
  begin
    DataDir := ExpandConstant('{commonappdata}\{#AppName}');
    if DirExists(DataDir) then
    begin
      if MsgBox('Also delete the ' + '{#AppName}' + ' data folder?' + #13#10 +
                DataDir + #13#10 + #13#10 +
                'This will permanently remove the database and all records. ' +
                'Choose No to keep your data (recommended).',
                mbConfirmation, MB_YESNO or MB_DEFBUTTON2) = IDYES then
        DelTree(DataDir, True, True, True);
    end;
  end;
end;
