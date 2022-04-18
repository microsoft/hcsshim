[CmdletBinding(SupportsShouldProcess)]
param (
    [string]
    # The path to the build file. Do not include `~` in the path
    $Path = '.\build.ninja',

    [string]
    $PwshSource = '',

    [string[]]
    $PwshFlags = '',

    [string]
    $GoSource = '',

    [string[]]
    $GoBuildFlags = '',

    [string[]]
    $GoTestFlags = '',

    [switch]
    $NoCrictl,

    [string]
    $CrictlSource = '',

    [string[]]
    $CrictlFlags = ("--image-endpoint 'tcp://127.0.0.1:2376'", "--runtime-endpoint 'tcp://127.0.0.1:2376'"),

    [string]
    $CPlatPath = 'C:\ContainerPlat',

    [string]
    $CPlatDataPath = 'C:\ContainerPlatData',

    [switch]
    $NoProto,

    [string]
    $ProtobuildSource = '',

    [string]
    # the version of protobuild to download in the ninja file
    $ProtobuildVersion = '0.2.0',

    [string[]]
    $ProtobuildFlags = '',

    [string]
    $ProtocSource = '',

    [string]
    # the version of protoc to download in the ninja file
    $ProtocVersion = '3.19.4',

    [string]
    $ProtocCtrdSource = '',

    [switch]
    $Info
)

# todo: `Install-Module -Name PSScriptAnalyzer` and run on .\scripts\*
# todo: absolute paths everythwhere -  https://github.com/ninja-build/ninja/issues/1251
# todo: split up into sub-ninjas for misc and proto

# ninja strips leading `.` and trailing slashes from relative
# paths for $in and $out, but not for other variables.
# So, for now, build targets and dependencies will all be relative to package root

Import-Module ( Join-Path $PSScriptRoot NinjaBuild.psm1 ) -Force

if ( $Info ) {
    Get-Help Update-NinjaFile

    Write-Output '' ''

    (Get-Command Update-NinjaFile).ParameterSets |
        Select-Object -Property @{n = 'ParameterSetName'; e = { $_.name } },
        @{n = 'Parameters'; e = { $_.ToString() } }

    return
}

####################################################################################################
# setup
####################################################################################################

Write-Verbose 'resolving paths and commands'

if ( -not (Test-Path $Path -IsValid) ) {
    throw "`"$Path`" is not a valid path"
}
if ( -not ( [System.IO.Path]::IsPathRooted($Path) ) ) {
    $Path = Join-Path (Get-Location) $Path
}
$Path = [IO.Path]::GetFullPath($Path)
Write-Verbose "using build file path `"$Path`""

$GoModule = Get-GoModuleName -Path '.'
if ( -not $GoModule ) {
    throw "Could not retreive the go module name in $(Get-Item '.')"
}
Write-Verbose "Found module name `"$GoModule`""

$GoPath = Get-GoEnv 'GOPATH'
Write-Verbose "Using GOPTH `"$GoPath`""

$Bin = 'bin'
$CmdsBin = "$Bin\cmd"
$ToolsBin = "$Bin\tools"
$TestsBin = "$Bin\test"
$OutDir = 'out'
$dirs = [string[]]($CmdsBin, $ToolsBin, $TestsBin, $OutDir)

$PwshSource = Resolve-Command -Name 'pwsh.exe' -Path $PwshSource
Write-Verbose "Using powershell `"$PwshSource`""

$GoSource = Resolve-Command -Name 'go.exe' -Path $GoSource
Write-Verbose "Using go `"$GoSource`""

####################################################################################################
# crictl and move setup
####################################################################################################

$moverules = $True

try {
    $CPlatPath = Resolve-PathError -Path $CPlatPath -Name 'Container Platform installation directory'
    Write-Verbose "Using Container Platform installation directory `"$CPlatPath`""
    $CPlatDataPath = Resolve-PathError $CPlatDataPath -Name 'Container Platform data directory'
    Write-Verbose "Using Container Platform data directory `"$CPlatDataPath`""
} catch {
    Write-Warning "$_ Skipping move rules."
    $moverules = $False
}

if ( -not $NoCrictl ) {
    try {
        $CrictlSource = Resolve-Command -Name 'crictl.exe' -Path $CrictlSource
        Write-Verbose "Using crictl `"$CrictlSource`""
    } catch {
        Write-Warning "$_ Skipping crictl rules."
        $NoCrictl = $True
    }
}

####################################################################################################
# proto setup
####################################################################################################

$ProtoBin = 'bin\protobuf'
$ProtoDir = 'protobuf'

$ProtocZip = "$OutDir\protoc.zip"
$LocalProtocSource = "$ProtoBin\protoc.exe"
$LocalProtobuildSource = "$GoPath\bin\protobuild.exe"

$installprotoc = $False
$installprotobuild = $False

if ( -not $NoProto ) {
    foreach ( $p in ( @{
                N = 'protobuild.exe'; S = ([ref]$ProtobuildSource); V = $ProtobuildVersion;
                I = ([ref]$installprotobuild) ; L = $LocalProtobuildSource
            },
            @{
                N = 'protocs.exe'; S = ([ref]$ProtocSource); V = $ProtocVersion;
                I = ([ref]$installprotoc) ; L = $LocalProtocSource
            }) ) {
        try {
            $v = Resolve-Command -Name $p['N'] -Path $p['S'].Value
            $p['S'].Value = ([string]$v)
            Write-Verbose "Using $($p['N']) `"$($p['S'].Value)`""
        } catch {
            $v = $p['V']
            if ( -not $v ) {
                throw "$($p['N']) version cannot be empty if source (`"$($p['S'].Value)`") is invalid"
            }
            Write-Warning "$_ Adding rules to install `"$($p['N'])`" (v$v)."
            # force the source to the local installation location, and flag it for installation
            $p['S'].Value = ([string]$p['L'])
            $p['I'].Value = $True
        }
    }

    # todo: when "go install github.com/containerd/containerd/cmd/protoc-gen-gogoctrds@latest" works, move this up
    try {
        $ProtocCtrdSource = Resolve-Command -Name 'protoc-gen-gogoctrd' -Path $ProtocCtrdSource
        Write-Verbose "Using protoc-gen-gogoctrd `"$ProtocCtrdSource`""
    } catch {
        Write-Error "$_ (Its a pain to install automatically.) Skipping protobuf rules."
        $NoProto = $True
    }
}

if ( -not $NoProto ) {
    $dirs = [string[]]( $dirs + $ProtoBin + $ProtoDir )
}

####################################################################################################
# preamble
####################################################################################################

Write-Verbose "generating build file`"$Path`""

'' | Out-File -FilePath $Path -NoNewline

$Path |
    Update-NinjaFile -Comment |
    Update-NinjaFile -Comment ninja.build for $GoModule |
    Update-NinjaFile -Comment |
    Update-NinjaFile -Comment This file is autogenerated. Do not edit. |
    Update-NinjaFile -Comment Created by $PSCommandPath |
    Update-NinjaFile -Comment Created on $(Get-Date) -NewLine |
    Update-NinjaFile -Variable ninja_required_version 1.10 -NewLine -q

####################################################################################################
# powershell
####################################################################################################

Write-Verbose 'adding powershell variables'

$PwshVar = 'PWSH'
$PwshFlags = ('-NoProfile', '-NoLogo', '-NonInteractive') + $PwshFlags
$PwshFlagsVar = 'PWSH_FLAGS'
$PwshCmd = [string[]](fv $PwshVar -q '"') + $PwshFlags + (fv $PwshFlagsVar) + '-Command'

$Path |
    Update-NinjaFile -Variable $PwshVar $PwshSource |
    Update-NinjaFile -Variable $PwshFlagsVar |
    Update-NinjaFile -NewLine -q

####################################################################################################
# misc
####################################################################################################

Write-Verbose 'adding miscellaneous rules and builds'

$CmdFlagsVar = 'CMD_FLAGS'
$SourceVar = 'SOURCE'
$DestVar = 'DESTINATION'
$UrlVar = 'URL'
$VersionVar = 'VERSION'

$Path |
    Update-NinjaFile -Comment |
    Update-NinjaFile -Comment miscellaneous utilities |
    Update-NinjaFile -Comment -NewLine |

    # variables

    Update-NinjaFile -Variable $CmdFlagsVar |
    Update-NinjaFile -Variable $DestVar |
    Update-NinjaFile -Variable $UrlVar |
    Update-NinjaFile -Variable $VersionVar |
    Update-NinjaFile -NewLine |

    # unzip

    Update-NinjaFile -Rule unzip `
        -Description ('unziping "$in" to', (fv $DestVar `"), 'with flags', (fv $CmdFlagsVar `")) `
        @PwshCmd 'Expand-Archive' '-Force' '-DestinationPath' (fv $DestVar "'") `
    (fv $CmdFlagsVar) '''$in''' |
    Update-NinjaFile -NewLine |

    # tar

    Update-NinjaFile -Rule tar `
        -Description ('taring "$in" with flags', (fv $CmdFlagsVar `")) `
        @PwshCmd 'tar' '-f' '''$in''' (fv $CmdFlagsVar) |
    Update-NinjaFile -NewLine |

    # download

    Update-NinjaFile -Rule web-download `
        -Description ('downloading "$out" from ', (fv $UrlVar `"), 'with flags', (fv $CmdFlagsVar `")) `
        @PwshCmd 'Invoke-WebRequest' '-Method GET' '-OutFile ''$out''' '-Uri' (fv $UrlVar `') |
    Update-NinjaFile -NewLine |

    # todo: when the build rules are updated, change this to $in vs $Source
    Update-NinjaFile -Rule mv `
        -Description ('moving "$in" to', (fv $DestVar '"'), 'with flags', (fv $CmdFlagsVar '"')) `
        @PwshCmd 'Move-Item' (fv $SourceVar "'") (fv $CmdFlagsVar) (fv $DestVar "'") |
    Update-NinjaFile -NewLine |

    # mkdir

    Update-NinjaFile -Rule mkdir `
        -Description ('creating directory $out with flags', (fv $CmdFlagsVar '"')) `
        @PwshCmd '(Test-Path' '-PathType Container' '-Path ''$out'')' '-or' `
        '(New-Item ''$out''' $(fv $CmdFlagsVar) '-ItemType Directory)' '> $$null' |
    Update-NinjaFile -NewLine |

    # rmdir

    Update-NinjaFile -Comment Use (fv $DestVar) rather than '$in' because the latter `
        would force the directory to be created if it did not exist |
    Update-NinjaFile -Rule clean `
        -Description ('removing directory', (fv $DestVar '"'), 'with flags', $(fv $CmdFlagsVar '"')) `
        @PwshCmd '(Test-Path' "-Path $(fv $DestVar "'"))" '-and' `
        '(Remove-Item' $(fv $DestVar "'") '-Recurse' '-Force' $(fv $CmdFlagsVar) ')' '> $$null' |
    Update-NinjaFile -NewLine -q

foreach ($dir in $dirs ) {
    $Path |
        Update-NinjaFile -Build $dir -Rule mkdir |
        Update-NinjaFile -Build "rm-$($dir.Replace('\','-'))" -Rule clean `
            -Variables @{$DestVar = $dir } |
        Update-NinjaFile -NewLine -q
}

Update-NinjaFile -Path $Path -Build clean -Rule phony @($dirs | ForEach-Object { "rm-$($_.Replace('\','-'))" }) -q

####################################################################################################
# go
####################################################################################################

Write-Verbose 'adding go variables, rules, and builds'

$GoFlags = [string[]]('')
$GoBuildFlags = (, "-ldflags='-s -w'") + $GoBuildFlags
$GoTestFlags = ("-gcflags='all=-d=checkptr'", '-tags functional') + $GoTestFlags

$GoVar = 'GO'
$GOOSVar = 'GOOS'
$GoFlagsVar = 'GO_FLAGS'

$GoCmd = [string[]]('&', (fv $GoVar `'))
$GoCmdEnv = [string[]](('$$env:GOOS=' + (fv $GOOSVar `')), ';', $GoCmd)
# pulls the exe ending based on GOOS
$GoExeCmd = ('&', (fv $GoVar `'), 'env GOEXE') -join ' '

$Path |
    Update-NinjaFile -Comment |
    Update-NinjaFile -Comment 'go' |
    Update-NinjaFile -Comment |
    Update-NinjaFile -Comment ('use fake targets for building and vendoring ' + `
            'since `go` is (fairly) quick, has its own cache, and understands test ' + `
            'dependencies, vendoring, etc.' -split ' ') `
        -NewLine |

    # variables

    Update-NinjaFile -Variable $GoVar $GoSource |
    Update-NinjaFile -Variable $GOOSVar windows |
    Update-NinjaFile -Variable $GoFlagsVar |
    Update-NinjaFile -NewLine |

    # install

    Update-NinjaFile -Comment install modules -NewLine |

    Update-NinjaFile -Rule go-install `
        -Description 'installing "$out" from', "`"$(fv $UrlVar)@$(fv $VersionVar)`"" `
        @PwshCmd @GoCmdEnv 'install' (fv $GoFlagsVar) `
        "'$(fv $UrlVar)@$(fv $VersionVar)'" |
    Update-NinjaFile -NewLine |

    # vendoring

    Update-NinjaFile -Comment vendoring -NewLine |

    Update-NinjaFile -Rule vendor `
        -Description 'tidying and vendoring $in' `
        @PwshCmd 'cd $in' ';' @GoCmd 'mod tidy' '-e' @GoFlags (fv $GoFlagsVar) ';' `
        @GoCmd 'mod vendor' '-e' @GoFlags (fv $GoFlagsVar) |
    Update-NinjaFile -NewLine |

    Update-NinjaFile -Build vend -Rule phony vendmain vendtest -Default |
    Update-NinjaFile -NewLine |

    Update-NinjaFile -Build vendmain -Rule vendor '.' |
    Update-NinjaFile -Build vendtest -Rule vendor .\test |
    Update-NinjaFile -NewLine -q

####################################################################################################
# go builds
####################################################################################################

# todo: break this up into "phony rule name", "exe name", "build source dir", "dest dir"

Write-Verbose 'adding go exe rules, and builds'

$builds = @(
    @{Build = 'shim'; Value = 'cmd\containerd-shim-runhcs-v1' }
    @{Build = 'runhcs'; Value = 'cmd\runhcs'; Variables = @{ $DestVar = $CmdsBin } }
    @{Build = 'ncproxy'; Value = 'cmd\ncproxy'; Variables = @{ $DestVar = $CmdsBin } }
    @{Build = 'wclayer'; Value = 'cmd\wclayer'; Variables = @{ $DestVar = $CmdsBin } }
    @{Build = 'tar2ext4'; Value = 'cmd\tar2ext4'; Variables = @{ $DestVar = $CmdsBin } }
    @{Build = 'shimdiag'; Value = 'cmd\shimdiag'; Variables = @{ $DestVar = $CmdsBin } }

    @{Build = 'uvmboot'; Value = 'internal\tools\uvmboot'; Variables = @{ $DestVar = $ToolsBin } }
    @{Build = 'zapdir'; Value = 'internal\tools\zapdir'; Variables = @{ $DestVar = $ToolsBin } }

    @{Build = 'gcs'; Value = 'cmd\gcs'; Variables = @{ $DestVar = $CmdsBin; $GOOSVar = 'linux' } }
)

$Path |
    Update-NinjaFile -Comment go builds -NewLine |

    Update-NinjaFile -Rule 'go-build' `
        -Description ('building', ('".\' + (fv $DestVar) + '\$out"'), 'from ".\$in"', `
            'with flags', "`"GOOS=$(fv $GOOSVar -q `')", (fv $GoFlagsVar -r `")) `
        @PwshCmd @GoCmdEnv 'build' @GoBuildFlags @GoFlags (fv $GoFlagsVar) '-o' `
        ".\$(fv $DestVar)" '.\$in' |
    Update-NinjaFile -NewLine |

    Update-NinjaFile -Build tools -Rule phony uvmboot zapdir tar2ext4 shimdiag |
    Update-NinjaFile -NewLine -q

foreach ( $build in $builds ) {
    $loc = $build['Variables']?[$DestVar]
    if ( $loc ) {
        $build['OrderOnly'] = @($build['OrderOnly'], $loc)
    }

    Update-NinjaFile -Path $Path -Rule 'go-build' @build -q
}

Update-NinjaFile -Path $Path -NewLine -q

if ( $moverules ) {
    $moves = @( @{Name = 'shim'; Exe = 'containerd-shim-runhcs-v1.exe' } )

    foreach ( $mv in $moves ) {
        Update-NinjaFile -Path $Path -Build "mv-$($mv['Name'])" -Rule mv `
            -Implicit (($NoCrictl) ? '' :  'rmpods') `
            -Value ($mv['Name']) `
            -Variables @{
            $SourceVar   = ($mv['Exe'])
            $DestVar     = (Join-Path $CPlatPath ($mv['Dest'] ?? '') $mv['Exe'])
            $CmdFlagsVar = '-Force'
        } -q
    }
}
Update-NinjaFile -Path $Path -NewLine -q

####################################################################################################
# go test builds
####################################################################################################

Write-Verbose 'adding go test exe rules, and builds'

$tests = @(
    @{Build = 'shimtest'; Value = 'test\containerd-shim-runhcs-v1'; Variables = @{ $DestVar = $TestsBin } }
    @{Build = 'critest'; Value = 'test\cri-containerd'; Variables = @{ $DestVar = $TestsBin } }
    @{Build = 'functional'; Value = 'test\functional'; Variables = @{ $DestVar = $TestsBin } }
    @{Build = 'runhcstest'; Value = 'test\runhcs'; Variables = @{ $DestVar = $TestsBin } }
    # @{Build = 'gcstest'; Value = 'test\gcs'; Variables = @{ $DestVar = $TestsBin; $GOOSVar = 'linux' } }
)

$Path |
    Update-NinjaFile -Comment go test builds |
    Update-NinjaFile -Comment '`go build`' accepts '`-o <directory>`,' but '`test -c`' requires '`-o <filename>`' |
    Update-NinjaFile -NewLine |


    Update-NinjaFile -Rule 'go-build-test' `
        -Description ('building test binary', ('".\' + (fv $DestVar) + '\$out"'), `
            'from ".\$in"', 'with flags', "`"GOOS=$(fv $GOOSVar -q `')", `
        (fv $GoFlagsVar -r `")) `
        @PwshCmd @GoCmdEnv 'test' @GoTestFlags @GoFlags (fv $GoFlagsVar) `
        '-o' ".\$(fv $DestVar)\`$out`$`$($GoExeCmd)" '-c .\$in' |
    Update-NinjaFile -NewLine |

    Update-NinjaFile -Build tests -Rule phony @($tests | ForEach-Object { $_['Build'] }) |
    Update-NinjaFile -NewLine -q

foreach ( $test in $tests ) {
    $loc = $test['Variables']?[$DestVar]
    if ( $loc ) {
        $test['OrderOnly'] = @($test['OrderOnly'], $loc)
    }

    Update-NinjaFile -Path $Path -Rule 'go-build-test' @test -q
}
Update-NinjaFile -NewLine -Path $Path -q

####################################################################################################
# go generate
####################################################################################################

Write-Verbose 'adding go generate rules, and builds'

$gogen_deps = @(
    @{URL = 'github.com/josephspurrier/goversioninfo/cmd/goversioninfo'; Version = 'latest' }
)

$Path |
    Update-NinjaFile -Comment go generate |
    Update-NinjaFile -Comment ('We dont really know what `go generate` will output ' + `
            '(ie, if it will create a new file or update existing ones) so use ' + `
            'phony targets to specify the directories' -split ' ') `
        -NewLine -q

foreach ( $d in $gogen_deps ) {
    $n = Split-Path $d['URL'] -Leaf
    $d['Name'] = $n
    $d['Path'] = Join-Path $GoPath 'bin' ($n + (Get-GoEnv 'GOEXE'))

    Update-NinjaFile -Path $Path -Build $d['Path'] -Rule go-install `
        -Variables @{$UrlVar = $d['URL'] ; $VersionVar = $d['Version'] } -q
}

$gogenOO = $gogen_deps | ForEach-Object { $_['Path'] }
Update-NinjaFile -Path $Path -NewLine -q

$Path |
    Update-NinjaFile -Rule 'go-gen' `
        -Description ('calling go generate on package ".\$in"', 'with flags', `
            "`"GOOS=$(fv $GOOSVar -q `')", (fv $GoFlagsVar -r `")) `
        @PwshCmd @GoCmdEnv 'generate' @GoFlags (fv $GoFlagsVar) '.\$in' |
    Update-NinjaFile -NewLine -q

$gens = Get-GoGenPackage '.' | ForEach-Object {
    @{ Name = "gogen-$($_.Replace('\','-'))"; Dir = $_ }
}

Update-NinjaFile -Path $Path -Build go-gen -Rule phony @($gens | ForEach-Object { $_['Name'] }) -NewLine -q

foreach ($d in $gens ) {
    Update-NinjaFile -Path $Path -Rule 'go-gen' -Build $d['Name'] -OrderOnly $gogenOO $d['Dir'] -q
}
Update-NinjaFile -Path $Path -NewLine -q

####################################################################################################
# crictl
####################################################################################################

if ( -not $NoCrict ) {
    $CrictlVar = 'CRICTL'
    $CrictlFlagsVar = 'CRICTL_FLAGS'

    $CrictlCmd = "& $(fv $CrictlVar "'")"

    $Path |
        Update-NinjaFile -Comment |
        Update-NinjaFile -Comment crictl |
        Update-NinjaFile -Comment |
        Update-NinjaFile -NewLine |

        # variables

        Update-NinjaFile -Variable $CrictlVar $CrictlSource |
        Update-NinjaFile -NewLine |

        # crictl commands

        Update-NinjaFile -Rule rmpods `
            -Description ('removing CRI podswith flags', "`"$CrictlFlags`"", (fv $CrictlFlagsVar `")) `
            @PwshCmd $CrictlCmd (fv $CrictlFlagsVar) 'pods --quiet' '| ForEach-Object' `
            "{ $CrictlCmd" $CrictlFlags 'stopp' (fv $CrictlFlagsVar) '$$_' ';' `
            $CrictlCmd $CrictlFlags 'rmp' (fv $CrictlFlagsVar) '--force' '$$_}' |
        Update-NinjaFile -NewLine |
        Update-NinjaFile -Build rmpods -Rule rmpods |
        Update-NinjaFile -NewLine -q
}

####################################################################################################
# proto
####################################################################################################

if ( -not $NoProto ) {
    Write-Verbose 'adding protobuf variables, rules, and builds'

    $ProtobuildVar = 'PROTOBUILD'
    $ProtobuildFlagsVar = 'PROTOBUILD_FLAGS'

    $ProtoCmd = "& $(fv $ProtobuildVar "'")"

    [string[]]$ProtoEnv = ''
    $Path |
        Update-NinjaFile -Comment |
        Update-NinjaFile -Comment protobuild |
        Update-NinjaFile -Comment |
        Update-NinjaFile -NewLine |

        # variables

        Update-NinjaFile -Variable $ProtobuildVar $ProtobuildSource |
        Update-NinjaFile -Variable $ProtobuildFlagsVar $ProtobuildFlags |
        Update-NinjaFile -NewLine -q

    # install dependencies

    if ( $installprotobuild ) {
        Write-Verbose 'adding protobuild installation build'

        $ProtobuildModule = 'github.com/containerd/protobuild'

        $Path |
            Update-NinjaFile -Comment install protobuild -NewLine |

            Update-NinjaFile -Build (fv $ProtobuildVar) -Rule go-install `
                -Variables @{$UrlVar = $ProtobuildModule ; $VersionVar = "v$ProtobuildVersion" } |
            Update-NinjaFile -NewLine -q

        $ProtoEnv += ('$$env:Path +=', "';$(Split-Path $ProtobuildSource )'", ';')
    }

    if ( $installprotoc ) {
        Write-Verbose 'adding protoc installation variables, rules, and build'

        $ProtocURL = 'https://github.com/protocolbuffers/protobuf/releases/download/' + `
            "v$ProtocVersion/protoc-$ProtocVersion-win32.zip"

        $Path |
            Update-NinjaFile -Comment download and install protoc -NewLine |

            # variables

            Update-NinjaFile -Build $ProtocZip -Rule web-download -OrderOnly (Split-Path $ProtocZip) `
                -Variables @{$UrlVar = $ProtocURL } |
            Update-NinjaFile -NewLine |

            Update-NinjaFile -Rule install-protoc `
                -Description 'unpacking "protoc.exe" and include files from "$in"' `
                @PwshCmd 'tar' '-f $in' (fv $CmdFlagsVar) '-xmv -C $PROTOC_BIN' '--strip-components 1' 'bin' `
                ';' 'tar' '-f $in' (fv $CmdFlagsVar) '-xmv -C $PROTOC_INCLUDE' '--strip-components 1' 'include' |
            Update-NinjaFile -NewLine |

            Update-NinjaFile -Build $ProtocSource -Rule install-protoc $ProtocZip `
                -Variables @{'PROTOC_BIN' = (Split-Path $ProtocSource); 'PROTOC_INCLUDE' = $ProtoDir } `
                -OrderOnly ($ProtoBin, $ProtoDir) |
            Update-NinjaFile -NewLine -q

        $ProtoEnv += ('$$env:Path +=', "';$(Split-Path (Join-Path (Resolve-Path .) $ProtocSource))'", ';')
    }

    # protobuild proper
    # need the protoenv variable set for local protobuild/c installs

    $protos = Get-ProtoFile -Module '.' |
        ForEach-Object { @{ Proto = $_.replace('.proto', '.pb.go'); Go = $_; Dir = (Split-Path -Parent $_) } }

    $Path |
        Update-NinjaFile -Rule protobuild `
            -Description ('building proto files "$MODULE" with flags', (fv $ProtobuildFlagsVar `")) `
            @PwshCmd @ProtoEnv $ProtoCmd (fv $ProtobuildFlagsVar) '$MODULE' |
        Update-NinjaFile -NewLine |

        Update-NinjaFile -Build proto -Rule phony @($protos | ForEach-Object { $_['Proto'] }) |
        Update-NinjaFile -NewLine -q

    $protos |
        ForEach-Object {
            $M = Get-GoModuleName -Path ".\$($_['Dir'])"
            Update-NinjaFile -Path $Path -Rule protobuild -Build $_['Proto'] -Implicit $_['Go'] `
                -OrderOnly (($installprotobuild ? (fv $ProtobuildVar) : ''), `
                ($installprotoc ? $ProtocSource : '')) `
                -Variables @{MODULE = $M } -q
        }
}
