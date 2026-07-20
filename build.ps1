$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Location $Root
New-Item -ItemType Directory -Force -Path bin | Out-Null
$env:Path = [System.Environment]::GetEnvironmentVariable("Path","Machine") + ";" + [System.Environment]::GetEnvironmentVariable("Path","User")

foreach ($cmd in @("controller","agent","netd","updater","cli")) {
  Write-Host "building pathweaver-$cmd..."
  go build -o "bin/pathweaver-$cmd.exe" "./cmd/pathweaver-$cmd"
}

Push-Location web
npm install
npm run build
Pop-Location
Write-Host "done: bin/ + web/dist/"
