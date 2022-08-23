# ex: .\scripts\Test-Functional.ps1 -Action Bench -Count 2 -BenchTime "2x"

[CmdletBinding()]
param (
    [ValidateSet('Test', 'Bench', 'List')]
    [alias('a')]
    [string]
    $Action = 'Bench',

    [string]
    $Note = '',

    [string]
    $OutDirectory = '.\test\results',

    # test parameters
    [int]
    $Count = 1,

    [string]
    $BenchTime = '5s',

    [string]
    $Timeout = '10m',

    [alias('tv')]
    [switch]
    $TestVerbose,

    [string]
    $Run = '',

    [string]
    $Feature = ''
)

Import-Module ( Join-Path $PSScriptRoot Testing.psm1 ) -Force

$date = Get-Date
$testcmd, $out = New-TestCommand `
    -Action $Action `
    -Path .\bin\test\functional.exe `
    -Name functional `
    -OutDirectory $OutDirectory `
    -Date $date `
    -Note $Note `
    -TestVerbose:$TestVerbose `
    -Count $Count `
    -BenchTime $BenchTime `
    -Timeout $Timeout `
    -Run $Run `
    -Feature $Feature `
    -Verbose:$Verbose

Invoke-TestCommand `
    -TestCmd $testcmd `
    -OutputFile $out `
    -OutputCmd (&{ if ( $Action -eq 'Bench' ) { 'benchstat' } }) `
    -Preamble `
    -Date $Date `
    -Note $Note `
    -Verbose:$Verbose
