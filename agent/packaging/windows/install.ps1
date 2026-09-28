param(
  [Parameter(Mandatory = $true)][string]$JoinSecret,
  [string]$Invite = "",
  [string]$Name = "",
  [ValidateSet("worker","captain")][string]$Mode = "worker",
  [string]$AgentExe = ".\tesla-agent.exe",
  [switch]$ExposeApi
)

$ErrorActionPreference = "Stop"
Write-Host "Pirate Fleet installer ($Mode)"

$args = @("enroll", "--join-secret", $JoinSecret, "--mode", $Mode)
if ($Invite) { $args += @("--invite", $Invite) }
if ($Name) { $args += @("--name", $Name) }
& $AgentExe @args
if ($LASTEXITCODE -ne 0) { throw "enroll failed" }

& $AgentExe service install
& $AgentExe service start
Write-Host "Deckhand/Captain service started. Captain UI default http://127.0.0.1:7842 if mode=captain (run manually with --expose-api for API)."
