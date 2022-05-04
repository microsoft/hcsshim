$FOUND = 0
Get-ChildItem -Path $args[0] -Recurse -Include '*.go' | Where-Object { $_.Directory -NotMatch 'vendor|internal' } | ForEach-Object {
    $LISTDATA = (go list -tags functional -json $_.FullName | ConvertFrom-Json)
    $INTERNALS = $LISTDATA.Imports.Where({ $_.StartsWith("github.com/Microsoft/hcsshim/internal") }) + $LISTDATA.TestImports.Where({ $_.StartsWith("github.com/Microsoft/hcsshim/internal") })
    if ($INTERNALS.Length -gt 0) { Write-Output "$_ contains one or more imports from github.com/Microsoft/hcsshim/internal/..."; $FOUND += 1 }
}
exit $FOUND
