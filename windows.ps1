$VSN = "1.4.0"

# build the filename for the Zip archive and exe file
$FileName = "mmdbctl_$($VSN)_windows_amd64"
$ZipFileName = "$($FileName).zip"

# download and extract zip
Invoke-WebRequest -Uri "https://github.com/ipinfo/mmdbctl/releases/download/mmdbctl-$VSN/$FileName.zip" -OutFile ./$ZipFileName
Unblock-File ./$ZipFileName
Expand-Archive -Path ./$ZipFileName  -DestinationPath $env:LOCALAPPDATA\mmdbctl -Force

# delete if already exists
if (Test-Path "$env:LOCALAPPDATA\mmdbctl\mmdbctl.exe") {
  Remove-Item "$env:LOCALAPPDATA\mmdbctl\mmdbctl.exe"
}
Rename-Item -Path "$env:LOCALAPPDATA\mmdbctl\$FileName.exe" -NewName "mmdbctl.exe"

# setting up env. 
$PathContent = [Environment]::GetEnvironmentVariable('path', 'Machine')
$mmdbctlPath = "$env:LOCALAPPDATA\mmdbctl"

# if Path already exists
if ($PathContent -ne $null) {
  if (-Not($PathContent -split ';' -contains $mmdbctlPath)) {
    [System.Environment]::SetEnvironmentVariable("PATH", $Env:Path + ";$env:LOCALAPPDATA\mmdbctl", "Machine")
  }
}
else {
  [System.Environment]::SetEnvironmentVariable("PATH", $Env:Path + ";$env:LOCALAPPDATA\mmdbctl", "Machine")
}

# cleaning files
Remove-Item -Path ./$ZipFileName
"You can use mmdbctl now."