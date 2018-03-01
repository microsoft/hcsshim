<#
.NOTES
    Summary: Simple wrapper to build a local initrd.img from sources and optionally install it.

    License: See https://github.com/Microsoft/opengcs/blob/master/LICENSE

.Parameter Install
    Installs the built initrd.img  

#>


param(
    [Parameter(Mandatory=$false)][switch]$Install
)

$ErrorActionPreference = 'Stop'

function New-TemporaryDirectory {
    $parent = [System.IO.Path]::GetTempPath()
    [string] $name = [System.Guid]::NewGuid()
    New-Item -ItemType Directory -Path (Join-Path $parent $name)
}

Try {
    Write-Host -ForegroundColor Yellow "INFO: Starting at $(date)`n"

    &docker build --platform=linux -t opengcs .
    if ( $LastExitCode -ne 0 ) {
        Throw "failed to build opengcs image"
    }
    
    Write-Host -ForegroundColor Yellow "INFO: Copying initrd.img from opengcs:latest image"
    $d=New-TemporaryDirectory
    docker run --rm -v $d`:/out opengcs cp /initrd.img /out
    if ( $LastExitCode -ne 0 ) {
        Throw "failed to copy initrd.img to $d"
    }
    $generated = "$d`\initrd.img"
    $size=(Get-Item $generated).Length
    Write-Host -ForegroundColor Yellow "INFO: Created $generated"
    Write-Host -ForegroundColor Yellow "INFO: Size $size bytes"
    
    if ($Install) {
        if (Test-Path "C:\Program Files\Linux Containers\initrd.img" -PathType Leaf) {
            copy "C:\Program Files\Linux Containers\initrd.img" "C:\Program Files\Linux Containers\initrd.old"
            Write-Host -ForegroundColor Yellow "INFO: Backed up previous initrd.img to C:\Program Files\Linux Containers\initrd.old"
        }
        copy "$d`\initrd.img" "C:\Program Files\Linux Containers\initrd.img"
        Write-Host -ForegroundColor Yellow "INFO: Restart the docker daemon to pick up the new image"
    }
	
}
Catch [Exception] {
    Throw $_
}
Finally {
    Write-Host -ForegroundColor Yellow "INFO: Exiting at $(date)"
}
