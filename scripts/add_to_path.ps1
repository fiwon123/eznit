$currentPath = Get-Location | Select-Object -ExpandProperty Path

$oldPath = [Environment]::GetEnvironmentVariable("Path", "Machine")

if ($oldPath -split ';' -contains $currentPath) {
    Write-Host "Path is already present in the System variable." -ForegroundColor Yellow
} else {
    $newPath = "$oldPath;$currentPath"

    [Environment]::SetEnvironmentVariable("Path", $newPath, "Machine")

    Write-Host "Successfully added $currentPath to the System PATH." -ForegroundColor Green
}
