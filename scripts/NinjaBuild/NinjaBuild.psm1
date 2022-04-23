
# . "$PSScriptRoot\BuildRules.ps1"

# global variables for commands
foreach ($d in @(
        @{ N = 'In' ; V = '$in' }
        @{ N = 'Out' ; V = '$out' }

        # miscellaneous
        @{ N = 'CmdFlagsVar' ; V = 'CMD_FLAGS' }
        @{ N = 'DestVar' ; V = 'DESTINATION' }
        @{ N = 'ModuleVar' ; V = 'MODULE' }
        @{ N = 'NinjaModuleVar' ; V = 'NINJA_MODULE' }
        @{ N = 'SourceVar' ; V = 'SOURCE' }
        @{ N = 'StampVar' ; V = 'STAMP' }
        @{ N = 'StripCompVar' ; V = 'STRIP_COMPONENTS' }
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
        $ImplicitOutput = '',

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

        [string[]]
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
        Write-Warning "Could not resolve go build source path `"$Source`""
        return
    }

    $OrderOnly = [string[]]$OrderOnly + $Dest
    $Variables[$GOOSVar] = $GOOS
    $Variables[$DestVar] = Join-Path $Dest ((Split-Path $Source -Leaf) + $GoExe[$GOOS])
    $Variables[$SourceVar] = $src

    if ( $PSCmdlet.ShouldProcess($Path, "Adding go build statement `"$Rule`" for `"$Name`"") ) {
        Add-Build -Path $Path -Rule $Rule -Build $Name `
            -Implicit $Implicit -ImplicitOutput ($ImplicitOutput + $Variables[$DestVar]) `
            -DynDep $DynDep `
            -OrderOnly $OrderOnly -Variables $Variables `
            -Quiet
    }

    if ( $Move ) {
        $Move = Resolve-Path $Move
        if ( -not $src ) {
            Write-Warning "Could not resolve move destination path `"$Move`""
            return
        }

        $mvvars = @{
            $SourceVar   = $Variables[$DestVar]
            $DestVar     = (Join-Path $Move (Split-Path $Variables[$DestVar] -Leaf))
            $CmdFlagsVar = '-Force'
        }

        if ( $PSCmdlet.ShouldProcess($Path,
                "Adding build statement to move `"$Name`" to `"$($mvvars[$DestVar])`"") ) {
            Add-Build -Path $Path -Build "mv-$Name" -Rule $MoveRule `
                -Implicit ($MoveImplicit + $mvvars[$SourceVar]) `
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

    if ( -not $PSCmdlet.ShouldProcess($Path, 'Adding go variables and rules') ) {
        return $cmds
    }

    $GoBuildFlags = (, "-ldflags='-s -w'") + $GoBuildFlags
    $GoTestFlags = ("-gcflags='all=-d=checkptr'", '-tags functional') + $GoTestFlags

    $GoCmd = [string[]]('&', (fv $GoVar `'))
    $GoCmdEnv = [string[]](('$$env:GOOS=' + (fv $GOOSVar `')), ';' , "`n", $GoCmd)

    $Path |
        Add-Comment |
        Add-Comment 'go' |
        Add-Comment |
        Add-Comment ('use fake targets for building and vendoring ' + `
                'since `go` is (fairly) quick, has its own cache, and understands ' + `
                'test dependencies, vendoring, etc.' -split ' ') `
            -NewLine |

        # variables
        Add-Variable $GoVar $GoSource -NewLine -Quiet

    # install
    Write-Verbose 'Adding go install rule'
    $Path |
        Add-Comment install module -NewLine |
        Add-Rule $cmds.Install `
            -Description ('installing "$out" from', "`"$(fv $UrlVar)@$(fv $VersionVar)`"", `
                'with flags:', "GOOS=$(fv $GOOSVar -q `')", (fv $GoFlagsVar)) `
            @PwshCmd @GoCmdEnv 'install' (fv $GoFlagsVar) `
            "'$(fv $UrlVar)@$(fv $VersionVar)'" `
            -NewLine -Quiet

    # vendoring
    Write-Verbose 'Adding go vendering rule'
    $Path |
        Add-Comment vendor module -NewLine |
        Add-Rule $cmds.Vendor `
            -Description 'tidying and vendoring $in' `
            @PwshCmd 'Set-Location ''$in''' ';' "`n" `
            @GoCmd 'mod tidy' '-e' @GoFlags (fv $GoFlagsVar) ';' "`n" `
            @GoCmd 'mod vendor' '-e' @GoFlags (fv $GoFlagsVar) `
            -NewLine -Quiet

    # generate
    Write-Verbose 'Adding go generate rule'
    $Path |
        Add-Comment go generate |
        Add-Comment ('We dont really know what `go generate` will output ' + `
                '(ie, if it will create a new file or update existing ones) so use ' + `
                'fake targets to specify the directories' -split ' ') -NewLine |
        Add-Rule $cmds.Generate `
            -Description ('calling go generate on package ".\$in"', 'with flags:', `
                $GoFlags, "GOOS=$(fv $GOOSVar -q `')", (fv $GoFlagsVar)) `
            @PwshCmd @GoCmdEnv 'generate' @GoFlags (fv $GoFlagsVar) '.\$in' `
            -NewLine -Quiet

    # build
    Write-Verbose 'Adding go build rule'
    $Path |
        Add-Comment build go executable -NewLine |
        Add-Rule $cmds.Build `
            -Description ('building $out as', (fv $DestVar -q '"'), 'from', (fv $SourceVar '"'), `
                'with flags:', "$GoFlags" , "$GoTestFlags", `
                "GOOS=$(fv $GOOSVar -q `')", (fv $GoFlagsVar)) `
            @PwshCmd @GoCmdEnv 'build' @GoBuildFlags @GoFlags (fv $GoFlagsVar) `
            '-o' $(fv $DestVar -q "'") $(fv $SourceVar "'") `
            -NewLine -Quiet


    # test build
    Write-Verbose 'Adding test build rule'
    $Path |
        Add-Comment build go test executable |
        Add-NewLine |
        Add-Rule $cmds.TestBuild `
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

    if ( -not $PSCmdlet.ShouldProcess($Path, 'Adding crictl rules') ) {
        return $cmds
    }

    $CrictlCmd = "& $(fv $CrictlVar "'")"

    $Path |
        Add-Comment |
        Add-Comment crictl |
        Add-Comment -NewLine |

        # variables

        Add-Variable $CrictlVar $CrictlSource -NewLine |

        # crictl commands

        Add-Rule rmpods `
            -Description ('removing CRI pods with flags:', $Flags, (fv $CrictlFlagsVar)) `
            @PwshCmd `
            $CrictlCmd @CrictlFlags (fv $CrictlFlagsVar) 'pods' '--quiet' `
            '| ForEach-Object' '{' "`n" `
            $CrictlCmd @CrictlFlags 'stopp' (fv $CrictlFlagsVar) '$$_' ';' "`n" `
            $CrictlCmd @CrictlFlags 'rmp' (fv $CrictlFlagsVar) '--force' '$$_' `
            '}' -NewLine -Quiet

    $cmds
}

function Add-Self {
    [CmdletBinding(PositionalBinding = $False, SupportsShouldProcess)]
    [OutputType([string])]
    param (
        [Parameter(Mandatory)]
        [string[]]
        $PwshCmd,

        [string]
        $Location = (Get-Location),

        [Parameter(Mandatory)]
        [System.Management.Automation.InvocationInfo]
        $Invocation,

        [Parameter(ValueFromPipeline, ValueFromPipelineByPropertyName, Mandatory)]
        [Alias('p')]
        [string]
        # The ninja build file to update.
        $Path,

        $DebugPreference = $PSCmdlet.GetVariableValue('DebugPreference'),
        $VerbosePreference = $PSCmdlet.GetVariableValue('VerbosePreference'),
        $WhatIfPreference = $PSCmdlet.GetVariableValue('WhatIfPreference')
    )

    $s = $Invocation.MyCommand.Path
    $c = $Invocation.BoundParameters | Export-Parameter
    Write-Verbose "Script call: $s $c"

    if ( -not $PSCmdlet.ShouldProcess($Path, 'Adding ninja recreation rule and build statements') ) {
        return
    }

    $Path |
        Add-Rule configure -Generator `
            -Description ('Rebuilding NinjaFile with flags:', (fv $CmdFlagsVar)) `
            @PwshCmd 'Set-Location' (fv $DestVar -q "'") ';' `
            '$$d =' (fv $CmdFlagsVar -q "'") '|' 'ConvertFrom-Json' '-AsHashtable' ';' `
            '&' (fv $SourceVar -q "'") '@d' |
        Add-Build configure -Rule configure `
            -Variables @{
            $DestVar     = $Location
            $SourceVar   = $s
            $CmdFlagsVar = $c
        } -Quiet
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
        Script   = 'call-script'
        Unzip    = 'unzip'
        Tar      = 'tar'
        TarDD    = 'tar-dd'
        Download = 'web-download'
        Move     = 'mv'
        MakeDir  = 'mkdir'
        Remove   = 'rm'
    }

    if ( -not $PSCmdlet.ShouldProcess($Path, 'Adding miscellaneous rules') ) {
        return $cmds
    }

    $Path |
        Add-Comment |
        Add-Comment miscellaneous utilities |
        Add-Comment -NewLine |
        Add-Variable $NinjaModuleVar $PSScriptRoot -NewLine -Quiet

    # script

    Write-Verbose 'Adding script call rule'
    $Path |
        Add-Rule $cmds.Script `
            -Description ("script $(fv $SourceVar '"') with flags:", (fv $CmdFlagsVar)) `
            @PwshCmd '&' (fv $SourceVar -q "'") (fv $CmdFlagsVar) `
            -NewLine -Quiet

    # unzip

    Write-Verbose 'Adding unzip rule'
    $Path |
        Add-Rule $cmds.Unzip `
            -Description ('unziping "$in" to', (fv $DestVar `"), 'with flags:', (fv $CmdFlagsVar)) `
            @PwshCmd 'Expand-Archive' '-Force' '-DestinationPath' (fv $DestVar "'") (fv $CmdFlagsVar) `
            "'$In'" `
            -NewLine -Quiet

    # tar

    Write-Verbose 'Adding tar rules'
    $Path |
        Add-Rule $cmds.Tar `
            -Description ('tar "$in" (with stamp "$out") with flags:', (fv $CmdFlagsVar)) `
            @PwshCmd 'tar' '-f' "'$In'" (fv $CmdFlagsVar) '>' '(''$out'' ? ''$out'' $: $$null) ' `
            -NewLine -Quiet

    $tardd = @"
Import-Module $(fv $NinjaModuleVar -q "'") ;
`$`$fs = (tar -f '$in' -t '*.proto' | ForEach-Object {
Join-Path $(fv $DestVar -q "'") (`$`$_ -split '[/\\]', ($(fv $StripCompVar) + 1))[$(fv $StripCompVar)]
});
'$out' | New-DynDepFile -CreatedFor '$in' |
Add-Dyndep -Build $(fv $StampVar -q "'") -ImplicitOutput `$`$fs -Restat -Quiet
"@ -split "(`n)" -split ' '
    $Path |
        Add-Rule $cmds.TarDD `
            -Description @('dyndep file "$out" for stamp', (fv $StampVar -q '"')
            'with files in "$in" and flags:', (fv $CmdFlagsVar)) `
            @PwshCmd @tardd -NewLine -Quiet

    # download

    Write-Verbose 'Adding download rule'
    $Path |
        Add-Rule $cmds.Download `
            -Description ('downloading "$out" from ', (fv $UrlVar `"), 'with flags:', (fv $CmdFlagsVar)) `
            @PwshCmd 'Invoke-WebRequest' '-Method GET' '-OutFile ''$out''' '-Uri' (fv $UrlVar `') `
            -NewLine -Quiet

    # move

    Write-Verbose 'Adding move rule'
    $Path |
        Add-Rule $cmds.Move `
            -Description ('moving', (fv $SourceVar "'"), 'to', (fv $DestVar '"'), 'with flags:', (fv $CmdFlagsVar)) `
            @PwshCmd 'Move-Item' (fv $SourceVar "'") (fv $CmdFlagsVar) (fv $DestVar "'") `
            -NewLine -Quiet

    # make dir

    Write-Verbose 'Adding make dir rule'
    $Path |
        Add-Rule $cmds.MakeDir `
            -Description ('creating directory $out with flags:', (fv $CmdFlagsVar)) `
            @PwshCmd '(Test-Path' '-PathType Container' '-Path ''$out'')' '-or' `
            '(New-Item ''$out''' $(fv $CmdFlagsVar) '-ItemType Directory)' '> $$null' `
            -NewLine -Quiet

    # remove

    Write-Verbose 'Adding remove rule'
    $Path |
        Add-Comment Use (fv $SourceVar) rather than '$in' because the latter `
            would force the directory to be created if it did not exist |
        Add-Rule $cmds.Remove `
            -Description ('removing item', (fv $SourceVar '"'), 'with flags:', $(fv $CmdFlagsVar)) `
            @PwshCmd "`n" '(Test-Path' "-Path $(fv $SourceVar "'"))" '-and' "`n" `
            '(Remove-Item' $(fv $SourceVar "'") '-Recurse' '-Force' $(fv $CmdFlagsVar) ')' '> $$null' `
            -NewLine -Quiet

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

    if ( $PSCmdlet.ShouldProcess($Path, 'Adding powershell variables') ) {
        Add-Variable -Path $Path -Variable $PwshVar $Source -Quiet -Verbose:$Verbose
    }

    $cmd
}

function New-NinjaBuildFile {
    [CmdletBinding(PositionalBinding = $False, SupportsShouldProcess)]
    [OutputType([string])]
    param (
        [Parameter(Mandatory)]
        [string]
        $GoModule,

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
    if ( $PSCmdlet.ShouldProcess($Path, 'Creating new ninja build file') ) {
        '' | Out-File -FilePath $Path -NoNewline
    }

    if ( $PSCmdlet.ShouldProcess($Path, 'Adding ninja build header') ) {
        $Path |
            Add-Comment 'This file is autogenerated; DO NOT EDIT.' |
            Add-Comment |
            Add-Comment ninjabuild created for $GoModule |
            Add-Variable ninja_required_version $NinjaVersion -NewLine -Quiet
    }

    if ( -not $Quiet ) { $Path }
}

function New-DynDepFile {
    [CmdletBinding(PositionalBinding = $False, SupportsShouldProcess)]
    [OutputType([string])]
    param (
        [string]
        $CreatedFor = '',

        [string]
        $DynDepVersion = '1.0',

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
    if ( $PSCmdlet.ShouldProcess("$Path, Creating new dyndep ninja build file") ) {
        '' | Out-File -FilePath $Path -NoNewline
    }

    if ( $PSCmdlet.ShouldProcess($Path, 'Adding dyndep ninja build header') ) {
        $Path |
            Add-Comment 'This file is autogenerated; DO NOT EDIT.' |
            Add-Comment |
            Add-Comment dyndep ninja build file created for $CreatedFor |
            Add-Variable 'ninja_dyndep_version' $DynDepVersion -NewLine -Quiet
    }

    if ( -not $Quiet ) { $Path }
}
