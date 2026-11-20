package windowsadapter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// NativeWakeSpec is only the original stable launch selection. Task registration
// supplies no maintenance job grant, model authority or native execution proof.
type NativeWakeSpec struct {
	Name             string `json:"name"`
	Program          string `json:"program"`
	Arguments        string `json:"arguments"`
	WorkingDirectory string `json:"working_directory"`
	BindingSHA256    string `json:"binding_sha256"`
}

type NativeWakeStatus struct {
	State         string `json:"state"`
	Name          string `json:"name"`
	Present       bool   `json:"present"`
	Enabled       bool   `json:"enabled"`
	BindingSHA256 string `json:"binding_sha256"`
}

func (s NativeWakeSpec) Validate() error {
	if len(s.BindingSHA256) != 64 || strings.Trim(s.BindingSHA256, "0123456789abcdef") != "" || s.Name != "MaestroMaintenance_"+s.BindingSHA256 {
		return errors.New("exact owned native task identity required")
	}
	for _, value := range []string{s.Program, s.Arguments, s.WorkingDirectory} {
		if value == "" || len(value) > 16384 || strings.ContainsAny(value, "\x00\r\n") {
			return errors.New("closed native wake task fields required")
		}
	}
	return nil
}

// QuoteWakeArguments uses the Windows argv quoting rule without a command shell.
func QuoteWakeArguments(args []string) string {
	var out []string
	for _, arg := range args {
		var b strings.Builder
		b.WriteByte('"')
		slashes := 0
		for _, c := range arg {
			if c == '\\' {
				slashes++
				continue
			}
			if c == '"' {
				b.WriteString(strings.Repeat("\\", slashes*2+1))
				b.WriteRune(c)
			} else {
				b.WriteString(strings.Repeat("\\", slashes))
				b.WriteRune(c)
			}
			slashes = 0
		}
		b.WriteString(strings.Repeat("\\", slashes*2))
		b.WriteByte('"')
		out = append(out, b.String())
	}
	return strings.Join(out, " ")
}

func NativeWake(ctx context.Context, spec NativeWakeSpec, action string) (NativeWakeStatus, error) {
	if err := spec.Validate(); err != nil {
		return NativeWakeStatus{}, err
	}
	if action != "inspect" && action != "opt-in" && action != "pause" && action != "revoke" {
		return NativeWakeStatus{}, errors.New("closed native wake operation required")
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	body, err := json.Marshal(struct {
		Spec   NativeWakeSpec `json:"spec"`
		Action string         `json:"action"`
	}{spec, action})
	if err != nil {
		return NativeWakeStatus{}, err
	}
	output, err := runNativeWake(ctx, nativeWakeScript, body)
	if err != nil {
		return NativeWakeStatus{}, err
	}
	var status NativeWakeStatus
	decoder := json.NewDecoder(bytes.NewReader(output))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&status) != nil || status.Name != spec.Name || status.BindingSHA256 != spec.BindingSHA256 {
		return status, errors.New("native wake readback unavailable")
	}
	if status.State != "not_present" && status.State != "active" && status.State != "paused" {
		return status, errors.New("native wake state unsupported")
	}
	return status, nil
}

// Microsoft TaskFolder.RegisterTaskDefinition: TASK_CREATE=2, interactive token=3.
// A missing task is created without update/force flags. Existing tasks must match
// every fixed action/principal/cadence property before pause/resume/delete.
const nativeWakeScript = `$ErrorActionPreference='Stop'
[Console]::OutputEncoding=New-Object System.Text.UTF8Encoding($false)
[Console]::InputEncoding=New-Object System.Text.UTF8Encoding($false)
$r=[Console]::In.ReadToEnd()|ConvertFrom-Json
$s=$r.spec
if($s.name -cnotmatch '^MaestroMaintenance_[0-9a-f]{64}$' -or $s.name -cne ('MaestroMaintenance_'+$s.binding_sha256)){throw 'identity'}
$sid=[Security.Principal.WindowsIdentity]::GetCurrent().User.Value
$service=New-Object -ComObject 'Schedule.Service'
$service.Connect()
$folder=$service.GetFolder('\')
$task=$null
try{$task=$folder.GetTask($s.name)}catch{if($_.Exception.HResult -ne -2147024894){throw}}
function Assert-Owned($t){
 $d=$t.Definition
 $principal=[string]$d.Principal.UserId
 if($principal -notmatch '^S-1-'){$principal=(New-Object Security.Principal.NTAccount($principal)).Translate([Security.Principal.SecurityIdentifier]).Value}
 if($d.Data -cne ('Maestro/OSWK/'+$s.binding_sha256) -or $principal -ine $sid -or $d.Principal.LogonType -ne 3 -or $d.Principal.RunLevel -ne 0){throw 'foreign principal'}
 if($d.Actions.Count -ne 1 -or $d.Actions.Item(1).Type -ne 0){throw 'foreign action'}
 $a=$d.Actions.Item(1)
 if($a.Path -cne $s.program -or $a.Arguments -cne $s.arguments -or $a.WorkingDirectory -cne $s.working_directory){throw 'foreign arguments'}
 if($d.Triggers.Count -ne 1 -or $d.Triggers.Item(1).Type -ne 1 -or !$d.Triggers.Item(1).Enabled -or $d.Triggers.Item(1).Repetition.Interval -ne 'PT15M' -or [string]$d.Triggers.Item(1).Repetition.Duration -ne '' -or [string]$d.Triggers.Item(1).EndBoundary -ne '' -or $d.Triggers.Item(1).Repetition.StopAtDurationEnd){throw 'foreign trigger'}
 if($d.Settings.MultipleInstances -ne 2 -or $d.Settings.ExecutionTimeLimit -ne 'PT5M' -or !$d.Settings.StartWhenAvailable -or !$d.Settings.DisallowStartIfOnBatteries -or !$d.Settings.StopIfGoingOnBatteries){throw 'foreign settings'}
}
if($null -ne $task){Assert-Owned $task}
switch($r.action){
 'inspect'{}
 'opt-in'{
  if($null -eq $task){
   $d=$service.NewTask(0)
   $d.Data='Maestro/OSWK/'+$s.binding_sha256
   $d.RegistrationInfo.Author=$sid
   $d.RegistrationInfo.Description='Maestro owned maintenance discovery pulse; original job grants and idle remain required.'
   $d.Principal.UserId=$sid;$d.Principal.LogonType=3;$d.Principal.RunLevel=0
   $d.Settings.Enabled=$true;$d.Settings.MultipleInstances=2;$d.Settings.ExecutionTimeLimit='PT5M'
   $d.Settings.StartWhenAvailable=$true;$d.Settings.DisallowStartIfOnBatteries=$true;$d.Settings.StopIfGoingOnBatteries=$true
   $trigger=$d.Triggers.Create(1)
   $trigger.StartBoundary=(Get-Date).ToString('yyyy-MM-ddTHH:mm:sszzz')
   $trigger.Enabled=$true;$trigger.Repetition.Interval='PT15M';$trigger.Repetition.StopAtDurationEnd=$false
   $a=$d.Actions.Create(0);$a.Path=$s.program;$a.Arguments=$s.arguments;$a.WorkingDirectory=$s.working_directory
   $task=$folder.RegisterTaskDefinition($s.name,$d,2,$null,$null,3,$null)
   Assert-Owned $task
  }else{$task.Enabled=$true}
 }
 'pause'{if($null -ne $task){$task.Enabled=$false;$task.Stop(0)}}
 'revoke'{if($null -ne $task){$task.Enabled=$false;$task.Stop(0);$folder.DeleteTask($s.name,0);$task=$null}}
 default{throw 'unsupported action'}
}
if($null -eq $task){$state='not_present';$present=$false;$enabled=$false}else{
 $task=$folder.GetTask($s.name);Assert-Owned $task
 $present=$true;$enabled=[bool]$task.Enabled
 if($enabled){$state='active'}else{$state='paused'}
}
@{state=$state;name=$s.name;present=$present;enabled=$enabled;binding_sha256=$s.binding_sha256}|ConvertTo-Json -Compress
`
