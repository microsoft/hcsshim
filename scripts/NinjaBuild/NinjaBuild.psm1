
# global variables for commands
foreach ($d in @(
        # miscellaneous
        @{ N = 'CmdFlagsVar' ; V = 'CMD_FLAGS' }
        @{ N = 'DestVar' ; V = 'DESTINATION' }
        @{ N = 'ModuleVar' ; V = 'MODULE' }
        @{ N = 'SourceVar' ; V = 'SOURCE' }
        @{ N = 'UrlVar' ; V = 'URL' }
        @{ N = 'VersionVar' ; V = 'VERSION' }

        # powershell
        @{ N = 'PwshVar' ; V = 'PWSH' }
        @{ N = 'PwshFlagsVar' ; V = 'PWSH_FLAGS' }

        # go
        @{ N = 'GoVar' ; V = 'GO' }
        @{ N = 'GoFlagsVar' ; V = 'GO_FLAGS' }
        @{ N = 'GOOSVar' ; V = 'GOOS' }
        @{ N = 'GoExe' ; V = @{'windows' = '.exe' } }

        # crictl
        @{ N = 'CrictlVar' ; V = 'CRICTL' }
        @{ N = 'CrictlFlagsVar' ; V = 'CRICTL_FLAGS' }

        # protobuild
        @{ N = 'ProtobuildVar' ; V = 'PROTOBUILD' }
        @{ N = 'ProtobuildFlagsVar' ; V = 'PROTOBUILD_FLAGS' }
    ) ) {
    Set-Variable -Name $d['N'] -Value $d['V'] `
        -Scope Global -Option Readonly -Force `
        -ErrorAction SilentlyContinue
}

function Add-GoBuildDeclaration {
    [CmdletBinding(PositionalBinding = $False, SupportsShouldProcess)]
    [OutputType([string])]
    param (
        [Parameter(Mandatory)]
        [string]
        $Name,

        [Parameter(Mandatory)]
        [string]
        $Rule,

        [string[]]
        $Implicit = '',

        [string[]]
        $OrderOnly = '',

        [hashtable]
        $Variables = @{},

        [string[]]
        $ImplicitOutputs = '',

        [string]
        $DynDep = '',

        [Parameter(Mandatory)]
        [string]
        $Source,

        [string]
        $Dest = '',

        [string]
        $Move = '',

        [string]
        $MoveRule = 'mv',

        [string]
        $MoveImplicit = '',

        [ValidateSet('windows', 'linux')]
        [string]
        $GOOS = 'windows',

        [Parameter(ValueFromPipeline, ValueFromPipelineByPropertyName, Mandatory)]
        [Alias('p')]
        [string]
        # The ninja build file to update.
        $Path,

        $DebugPreference = $PSCmdlet.GetVariableValue('DebugPreference'),
        $VerbosePreference = $PSCmdlet.GetVariableValue('VerbosePreference'),
        $WhatIfPreference = $PSCmdlet.GetVariableValue('WhatIfPreference')
    )

    $src = Resolve-Path $Source 2> $null
    if ( -not $src ) {
        Write-Warning "Could not resolve path `"$Source`""
        return
    }

    $OrderOnly = [string[]]$OrderOnly + $Dest
    $Variables[$GOOSVar] = $GOOS
    $Variables[$DestVar] = (Join-Path ( Resolve-Path '.' ) $Dest (Split-Path $Source -Leaf)) + $GoExe[$GOOS]
    $Variables[$SourceVar] = $src

    if ( $PSCmdlet.ShouldProcess("Appending $Rule build declaration for `"$Name`" to ninja build file `"$Path`"", $Path, 'Update-NinjaFile') ) {
        Update-NinjaFile -Path $Path -Rule $Rule -Build $Name `
            -Implicit $Implicit -ImplicitOutputs $ImplicitOutputs `
            -DynDep $DynDep `
            -OrderOnly $OrderOnly -Variables $Variables `
            -Quiet
    }

    if ( $Move ) {
        $mvvars = @{
            $SourceVar   = $Variables[$DestVar]
            $DestVar     = (Join-Path (Resolve-Path $Move) (Split-Path $Variables[$DestVar] -Leaf))
            $CmdFlagsVar = '-Force'
        }

        if ( $PSCmdlet.ShouldProcess(
                "Adding move build declaration for `"$Name`" to `"$($mvvars['DestVar'])`" to ninja build file `"$Path`"",
                $Path, 'Update-NinjaFile') ) {
            Update-NinjaFile -Path $Path -Build "mv-$Name" -Rule $MoveRule `
                -Implicit ($Name, $MoveImplicit) `
                -Variables $mvvars `
                -Quiet
        }
    }

    $Name
}

function Add-GoRule {
    [CmdletBinding(PositionalBinding = $False, SupportsShouldProcess)]
    param (
        [Parameter(Mandatory)]
        [string[]]
        $PwshCmd,

        [Parameter(Mandatory)]
        [string]
        $GoSource,

        [string[]]
        $GoFlags = '',

        [string[]]
        $GoBuildFlags = '',

        [string[]]
        $GoTestFlags = '' ,

        [Parameter(ValueFromPipeline, ValueFromPipelineByPropertyName, Mandatory)]
        [Alias('p')]
        [string]
        # The ninja build file to update.
        $Path,

        $DebugPreference = $PSCmdlet.GetVariableValue('DebugPreference'),
        $VerbosePreference = $PSCmdlet.GetVariableValue('VerbosePreference'),
        $WhatIfPreference = $PSCmdlet.GetVariableValue('WhatIfPreference')
    )

    $cmds = [PSCustomObject]@{
        Install   = 'go-install'
        Vendor    = 'go-vend'
        Generate  = 'go-gen'
        Build     = 'go-build'
        TestBuild = 'go-build-test'
    }

    if ( -not $PSCmdlet.ShouldProcess("Adding go variables and rules to ninja build file `"$Path`"", $Path, 'Update-NinjaFile') ) {
        return $cmds
    }

    $GoBuildFlags = (, "-ldflags='-s -w'") + $GoBuildFlags
    $GoTestFlags = ("-gcflags='all=-d=checkptr'", '-tags functional') + $GoTestFlags

    $GoCmd = [string[]]('&', (fv $GoVar `'))
    $GoCmdEnv = [string[]](('$$env:GOOS=' + (fv $GOOSVar `')), ';', $GoCmd)

    $Path |
        Update-NinjaFile -Comment |
        Update-NinjaFile -Comment 'go' |
        Update-NinjaFile -Comment |
        Update-NinjaFile -Comment ('use fake targets for building and vendoring ' + `
                'since `go` is (fairly) quick, has its own cache, and understands ' + `
                'test dependencies, vendoring, etc.' -split ' ') `
            -NewLine |

        # variables
        Update-NinjaFile -Variable $GoVar $GoSource -NewLine -Quiet

    # install
    Write-Verbose 'Adding go install rule'
    $Path |
        Update-NinjaFile -Comment install module -NewLine |
        Update-NinjaFile -Rule $cmds.Install `
            -Description ('installing "$out" from', "`"$(fv $UrlVar)@$(fv $VersionVar)`"", `
                'with flags:', "GOOS=$(fv $GOOSVar -q `')", (fv $GoFlagsVar)) `
            @PwshCmd @GoCmdEnv 'install' (fv $GoFlagsVar) `
            "'$(fv $UrlVar)@$(fv $VersionVar)'" `
            -NewLine -Quiet

    # vendoring
    Write-Verbose 'Adding go vendering rule'
    $Path |
        Update-NinjaFile -Comment vendor module -NewLine |
        Update-NinjaFile -Rule $cmds.Vendor `
            -Description 'tidying and vendoring $in' `
            @PwshCmd 'cd $in' ';' @GoCmd 'mod tidy' '-e' @GoFlags (fv $GoFlagsVar) ';' `
            @GoCmd 'mod vendor' '-e' @GoFlags (fv $GoFlagsVar) `
            -NewLine -Quiet

    # generate
    Write-Verbose 'Adding go generate rule'
    $Path |
        Update-NinjaFile -Comment go generate |
        Update-NinjaFile -Comment ('We dont really know what `go generate` will output ' + `
                '(ie, if it will create a new file or update existing ones) so use ' + `
                'fake targets to specify the directories' -split ' ') -NewLine |
        Update-NinjaFile -Rule $cmds.Generate `
            -Description ('calling go generate on package ".\$in"', 'with flags:', `
                $GoFlags, "GOOS=$(fv $GOOSVar -q `')", (fv $GoFlagsVar)) `
            @PwshCmd @GoCmdEnv 'generate' @GoFlags (fv $GoFlagsVar) '.\$in' `
            -NewLine -Quiet

    # build
    Write-Verbose 'Adding go build rule'
    $Path |
        Update-NinjaFile -Comment build go executable -NewLine |
        Update-NinjaFile -Rule $cmds.Build `
            -Description ('building $out as', (fv $DestVar -q '"'), 'from', (fv $SourceVar '"'), `
                'with flags:', "$GoFlags" , "$GoTestFlags", `
                "GOOS=$(fv $GOOSVar -q `')", (fv $GoFlagsVar)) `
            @PwshCmd @GoCmdEnv 'build' @GoBuildFlags @GoFlags (fv $GoFlagsVar) `
            '-o' $(fv $DestVar -q "'") $(fv $SourceVar "'") `
            -NewLine -Quiet


    # test build
    Write-Verbose 'Adding test build rule'
    $Path |
        Update-NinjaFile -Comment build go test executable |
        Update-NinjaFile -NewLine |
        Update-NinjaFile -Rule $cmds.TestBuild `
            -Description ('building test binary $out as', (fv $DestVar -q '"'), `
                'from', (fv $SourceVar '"'), 'with flags:', $GoFlags, $GoTestFlags, `
                "GOOS=$(fv $GOOSVar -q `')", (fv $GoFlagsVar)) `
            @PwshCmd @GoCmdEnv 'test' @GoTestFlags @GoFlags (fv $GoFlagsVar) `
            '-o' $(fv $DestVar -q "'") '-c' $(fv $SourceVar "'") `
            -NewLine -Quiet

    $cmds
}

function Add-CrictlRule {
    [CmdletBinding(PositionalBinding = $False, SupportsShouldProcess)]
    param (
        [Parameter(Mandatory)]
        [string[]]
        $PwshCmd,

        [Parameter(Mandatory)]
        [string]
        $CrictlSource,

        [string[]]
        $CrictlFlags = '',

        [Parameter(ValueFromPipeline, ValueFromPipelineByPropertyName, Mandatory)]
        [Alias('p')]
        [string]
        # The ninja build file to update.
        $Path,

        $DebugPreference = $PSCmdlet.GetVariableValue('DebugPreference'),
        $VerbosePreference = $PSCmdlet.GetVariableValue('VerbosePreference'),
        $WhatIfPreference = $PSCmdlet.GetVariableValue('WhatIfPreference')
    )

    $cmds = [PSCustomObject]@{
        RemovePods = 'rmpods'
    }

    if ( -not $PSCmdlet.ShouldProcess("Adding crictl rules to ninja build file `"$Path`"", $Path, 'Update-NinjaFile') ) {
        return $cmds
    }

    $CrictlCmd = "& $(fv $CrictlVar "'")"

    $Path |
        Update-NinjaFile -Comment |
        Update-NinjaFile -Comment crictl |
        Update-NinjaFile -Comment -NewLine |

        # variables

        Update-NinjaFile -Variable $CrictlVar $CrictlSource  -NewLine |

        # crictl commands

        Update-NinjaFile -Rule rmpods `
            -Description ('removing CRI pods with flags:', $Flags, (fv $CrictlFlagsVar)) `
            @PwshCmd `
            $CrictlCmd @CrictlFlags (fv $CrictlFlagsVar) 'pods' '--quiet' `
            '| ForEach-Object' '{' `
            $CrictlCmd @CrictlFlags 'stopp' (fv $CrictlFlagsVar) '$$_' ';' `
            $CrictlCmd @CrictlFlags 'rmp' (fv $CrictlFlagsVar) '--force' '$$_' `
            '}' -NewLine -Quiet

    $cmds
}

function Add-MiscRule {
    [CmdletBinding(PositionalBinding = $False, SupportsShouldProcess)]
    param (
        [Parameter(Mandatory)]
        [string[]]
        $PwshCmd,

        [Parameter(ValueFromPipeline, ValueFromPipelineByPropertyName, Mandatory)]
        [Alias('p')]
        [string]
        # The ninja build file to update.
        $Path,

        $DebugPreference = $PSCmdlet.GetVariableValue('DebugPreference'),
        $VerbosePreference = $PSCmdlet.GetVariableValue('VerbosePreference'),
        $WhatIfPreference = $PSCmdlet.GetVariableValue('WhatIfPreference')
    )

    $cmds = [PSCustomObject]@{
        Unzip    = 'unzip'
        Tar      = 'tar'
        Download = 'web-download'
        Move     = 'mv'
        MakeDir  = 'mkdir'
        Remove   = 'rm'
    }

    if ( -not $PSCmdlet.ShouldProcess("Adding miscellaneous rules to ninja build file `"$Path`"", $Path, 'Update-NinjaFile') ) {
        return $cmds
    }

    $Path |
        Update-NinjaFile -Comment -WhatIfPreference $WhatIfPreference |
        Update-NinjaFile -Comment miscellaneous utilities -WhatIfPreference $WhatIfPreference |
        Update-NinjaFile -Comment -NewLine -WhatIfPreference $WhatIfPreference -Quiet

    # unzip
    Write-Verbose 'Adding unzip rule'
    $Path |
        Update-NinjaFile -Rule $cmds.Unzip `
            -Description ('unziping "$in" to', (fv $DestVar `"), 'with flags:', (fv $CmdFlagsVar)) `
            @PwshCmd 'Expand-Archive' '-Force' '-DestinationPath' (fv $DestVar "'") (fv $CmdFlagsVar) `
            '''$in''' `
            -WhatIfPreference $WhatIfPreference -NewLine -Quiet

    # tar
    Write-Verbose 'Adding tar rule'
    $Path |
        Update-NinjaFile -Rule $cmds.Tar `
            -Description ('taring "$in" with flags:', (fv $CmdFlagsVar)) `
            @PwshCmd 'tar' '-f' '''$in''' (fv $CmdFlagsVar) `
            -WhatIfPreference $WhatIfPreference -NewLine -Quiet


    # download
    Write-Verbose 'Adding download rule'
    $Path |
        Update-NinjaFile -Rule $cmds.Download `
            -Description ('downloading "$out" from ', (fv $UrlVar `"), 'with flags:', (fv $CmdFlagsVar)) `
            @PwshCmd 'Invoke-WebRequest' '-Method GET' '-OutFile ''$out''' '-Uri' (fv $UrlVar `') `
            -WhatIfPreference $WhatIfPreference -NewLine -Quiet

    # move
    Write-Verbose 'Adding move rule'
    $Path |
        Update-NinjaFile -Rule $cmds.Move `
            -Description ('moving "$in" to', (fv $DestVar '"'), 'with flags:', (fv $CmdFlagsVar)) `
            @PwshCmd 'Move-Item' (fv $SourceVar "'") (fv $CmdFlagsVar) (fv $DestVar "'") `
            -WhatIfPreference $WhatIfPreference -NewLine -Quiet

    # make dir
    Write-Verbose 'Adding make dir rule'
    $Path |
        Update-NinjaFile -Rule $cmds.MakeDir `
            -Description ('creating directory $out with flags:', (fv $CmdFlagsVar)) `
            @PwshCmd '(Test-Path' '-PathType Container' '-Path ''$out'')' '-or' `
            '(New-Item ''$out''' $(fv $CmdFlagsVar) '-ItemType Directory)' '> $$null' `
            -WhatIfPreference $WhatIfPreference -NewLine -Quiet

    # remove
    Write-Verbose 'Adding remove rule'
    $Path |
        Update-NinjaFile -Comment Use (fv $DestVar) rather than '$in' because the latter `
            would force the directory to be created if it did not exist `
            -WhatIfPreference $WhatIfPreference |
        Update-NinjaFile -Rule $cmds.Remove `
            -Description ('removing item', (fv $DestVar '"'), 'with flags:', $(fv $CmdFlagsVar)) `
            @PwshCmd '(Test-Path' "-Path $(fv $DestVar "'"))" '-and' `
            '(Remove-Item' $(fv $DestVar "'") '-Recurse' '-Force' $(fv $CmdFlagsVar) ')' '> $$null' `
            -WhatIfPreference $WhatIfPreference -NewLine -Quiet

    $cmds
}

function Add-PwshRule {
    [CmdletBinding(PositionalBinding = $False, SupportsShouldProcess)]
    [OutputType([string[]])]
    param (
        [string]
        $Source = '',

        [string[]]
        $Flags = '',

        [Parameter(ValueFromPipeline, ValueFromPipelineByPropertyName, Mandatory)]
        [Alias('p')]
        [string]
        # The ninja build file to update.
        $Path,

        $DebugPreference = $PSCmdlet.GetVariableValue('DebugPreference'),
        $VerbosePreference = $PSCmdlet.GetVariableValue('VerbosePreference'),
        $WhatIfPreference = $PSCmdlet.GetVariableValue('WhatIfPreference')
    )
    $cmd = ([string[]](fv $PwshVar -q '"') + $Flags + (fv $PwshFlagsVar) + '-Command') | Get-NonEmpty

    if ( $PSCmdlet.ShouldProcess("Adding powershell variables to ninja build file `"$Path`"", $Path, 'Update-NinjaFile') ) {
        Update-NinjaFile -Path $Path -Variable $PwshVar $Source -NewLine -Quiet
    }

    $cmd
}

function New-NinjaBuildFile {
    [CmdletBinding(PositionalBinding = $False, SupportsShouldProcess)]
    [OutputType([string])]
    param (
        [DateTime]
        $Date = (Get-Date),

        [Parameter(Mandatory)]
        [string]
        $GoModule,

        [string]
        $CreatedBy = ($Script:MyInvocation.MyCommand.Path),

        [string]
        $NinjaVersion = '1.10',

        [Parameter(ValueFromPipeline, ValueFromPipelineByPropertyName, Mandatory)]
        [Alias('p')]
        [string]
        # The ninja build file to update.
        $Path,

        [Alias('q')]
        [switch]
        # Supress returning the path.
        $Quiet,

        $DebugPreference = $PSCmdlet.GetVariableValue('DebugPreference'),
        $VerbosePreference = $PSCmdlet.GetVariableValue('VerbosePreference'),
        $WhatIfPreference = $PSCmdlet.GetVariableValue('WhatIfPreference')
    )
    if ( $PSCmdlet.ShouldProcess("Creating new ninja build file `"$Path`"", $Path, 'Out-File') ) {
        '' | Out-File -FilePath $Path -NoNewline
    }

    if ( $PSCmdlet.ShouldProcess("Adding header to ninja build file `"$Path`"", $Path, 'Update-NinjaFile') ) {
        $Path |
            Update-NinjaFile -Comment |
            Update-NinjaFile -Comment ninja.build for $GoModule |
            Update-NinjaFile -Comment |
            Update-NinjaFile -Comment This file is autogenerated. Do not edit. |
            Update-NinjaFile -Comment Created by $CreatedBy |
            Update-NinjaFile -Comment Created on $Date -NewLine |
            Update-NinjaFile -Variable ninja_required_version $NinjaVersion -NewLine -Quiet:$Quiet
    }
}
