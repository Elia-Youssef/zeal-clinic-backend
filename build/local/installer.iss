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
DefaultDirName={localappdata}\{#AppName}
DefaultGroupName={#AppName}
UninstallDisplayName={#AppName}
UninstallDisplayIcon={app}\App\icon.ico
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
Source: "output\ZealClinic.exe"; DestDir: "{app}\App"; Flags: ignoreversion
Source: "output\ZealUpdater.exe"; DestDir: "{app}\App"; Flags: ignoreversion
Source: "icon.ico"; DestDir: "{app}\App"; Flags: ignoreversion

[Dirs]
; Per-user data directory (holds clinic.db). Lives under the user's own
; LocalAppData, so no extra ACLs are needed: they already have full access.
Name: "{app}\Data"

[Icons]
Name: "{group}\{#AppName}"; Filename: "{app}\App\{#AppExeName}"; IconFilename: "{app}\App\icon.ico"
Name: "{group}\{cm:UninstallProgram,{#AppName}}"; Filename: "{uninstallexe}"
Name: "{autodesktop}\{#AppName}"; Filename: "{app}\App\{#AppExeName}"; IconFilename: "{app}\App\icon.ico"; Tasks: desktopicon
Name: "{userstartup}\{#AppName}"; Filename: "{app}\App\{#AppExeName}"; Parameters: "--startup"; IconFilename: "{app}\App\icon.ico"; Tasks: startupicon

[Run]
; Firewall: delete any stale rule from a previous install, then add the current one.
; The delete will fail silently on a fresh install (no rule yet), which is fine.
Filename: "netsh"; Parameters: "advfirewall firewall delete rule name=""{#AppName}"""; Flags: runhidden
Filename: "netsh"; Parameters: "advfirewall firewall add rule name=""{#AppName}"" dir=in action=allow protocol=TCP localport={#AppPort} program=""{app}\App\{#AppExeName}"" description=""{#AppName} backend (HTTP)"""; Flags: runhidden; StatusMsg: "Configuring Windows Firewall..."
; Initialise the database (creates the clinic.db under Data\ if absent).
Filename: "{app}\App\{#AppExeName}"; Parameters: "--seed-only --no-browser"; Flags: runhidden waituntilterminated; StatusMsg: "Initialising database..."
; Silent self-update path: relaunch the app as the original (non-elevated) user
; so it drops back to medium integrity. Runs only for /VERYSILENT installs (i.e.
; the in-app updater), never for an interactive install.
Filename: "{app}\App\{#AppExeName}"; Parameters: "--post-update"; Flags: nowait runasoriginaluser; Check: WizardSilent
; Final-page checkbox: launch the app (interactive installs only).
Filename: "{app}\App\{#AppExeName}"; Description: "Launch {#AppName} now"; Flags: nowait postinstall skipifsilent

[UninstallRun]
Filename: "netsh"; Parameters: "advfirewall firewall delete rule name=""{#AppName}"""; Flags: runhidden; RunOnceId: "DelFirewallRule"

[Code]
procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
var
  DataDir: String;
begin
  if CurUninstallStep = usPostUninstall then
  begin
    DataDir := ExpandConstant('{localappdata}\{#AppName}\Data');
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
