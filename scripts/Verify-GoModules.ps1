function newTemporaryDirectory {
    $parent = [System.IO.Path]::GetTempPath()
    [string] $name = [System.Guid]::NewGuid()
    New-Item -ItemType Directory -Path (Join-Path $parent $name)
}

function updateVendor {
    param (
        [string]$path
    )
    $currentPath = (Get-Location).Path
    Set-Location $path
    go mod vendor
    go mod tidy 
    Set-Location $currentPath
}

function matchingHashes {
    param (
        [string]$rootPath,
        [string]$tempPath
    )
    $rootHashes = Get-ChildItem -Recurse -Path $rootPath | foreach  {Get-FileHash -Path $_.FullName -Algorithm SHA256}
    if (-not $?) {
        return $false
    }
    $tempHashes = Get-ChildItem -Recurse -Path $tempPath | foreach  {Get-FileHash -Path $_.FullName -Algorithm SHA256}
    if (-not $?) {
        return $false
    }
    $diff = Compare-Object -ReferenceObject $rootHashes.Hash -DifferenceObject $tempHashes.Hash
    if ($diff.Count -eq 0) {
        return $true
    }
    return $false
}

$rootPath = $args[0]
$subdir = $args[1]

$tempDir = newTemporaryDirectory
Copy-Item -Path $rootPath/* -Destination $tempDir -Recurse
if (-not $?) {
    Remove-Item $tempDir -Recurse
    exit 1
}

updateVendor $tempDir/$subdir
if (-not $?) {
    Remove-Item $tempDir -Recurse
    exit 1
}

$hashesMatch = matchingHashes $rootPath/$subdir/vendor $tempDir/$subdir/vendor
if (-not $?) {
    Remove-Item $tempDir -Recurse
    exit 1
}

Remove-Item $tempDir -Recurse
if ($hashesMatch -ne $true) {
    exit 1
}
