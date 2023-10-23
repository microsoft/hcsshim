# Example of who to set the version/git information in ./internal/version so
# that the binaries will report the appropriate information.

$ErrorActionPreference = 'Continue'
$VerbosePreference = 'Continue'

$root = Split-Path -Path $PSScriptRoot -Parent
$dataDir = Join-Path $root 'internal/version/data'

function run([string]$cmd, [string[]]$params) {
    Write-Verbose "$cmd $params"
    $s = & $cmd @params 2>&1
    if ( $LASTEXITCODE -ne 0 ) {
        $err = "Command '$cmd $params' failed"
        if ( -not [string]::IsNullOrWhiteSpace("$s") ) {
            $err += ": $s"
        }
        Write-Warning $err
    } else {
        $s
    }
}

function writeto([string]$f, [string]$v) {
    if ( [string]::IsNullOrWhiteSpace($v) ) {
        return
    }
    $v = $v.Trim()

    Write-Output "${f}: $v"
    $v | Out-File -Encoding ascii -FilePath (Join-Path $dataDir $f) -NoNewline
}

$branch = run 'git' @('rev-parse', '--abbrev-ref', 'HEAD')
if ( $branch -ceq 'HEAD') {
    Write-Warning 'Detected detached head'
    $branch = run 'git' @('name-rev', '--name-only', 'HEAD')
    if ( [string]::IsNullOrWhiteSpace($branch) -or $branch -eq 'undefined' ) {
        Write-Warning "'git name-rev' failed: $branch"
        $branch = run 'git' @('branch', '-r', '--contains=HEAD') | Select-Object -First 1
    }
}
$branch = $branch -replace '^(origin|remotes/origin|refs/heads)/', ''
writeto 'BRANCH' $branch

$commit = run 'git' @('rev-parse', 'HEAD')
try {
    run 'git' @('diff', '--no-ext-diff', '--quiet', '--exit-code', 'HEAD')
    if ( $LASTEXITCODE -ne 0 ) {
        Write-Warning 'Dirty repo'
        $commit += '-dirty'
    }
} catch { }
writeto 'COMMIT' $commit

# don't use `--always`; we we're okay with an empty version since commit will likely be non-empty
$version = run 'git' @('describe', '--match=v[0-9]*', '--tags', '--dirty')
writeto 'VERSION' $version
