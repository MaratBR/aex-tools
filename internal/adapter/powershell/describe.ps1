# Reads the script at $env:AEX_SCRIPT_PATH without running it (the PowerShell parser only) and
# prints its help and parameters as base64 of UTF-8 JSON, which no console code page can garble.
$ErrorActionPreference = 'Stop'
$tokens = $null
$parseErrors = $null
$ast = [System.Management.Automation.Language.Parser]::ParseFile($env:AEX_SCRIPT_PATH, [ref]$tokens, [ref]$parseErrors)

function Get-Constant($expr) {
    if ($null -eq $expr) { return $null }
    try { return $expr.SafeGetValue() } catch { return $expr.Extent.Text }
}

function Get-Kind([type]$type) {
    if ($null -eq $type) { return 'string' }
    if ($type -eq [switch]) { return 'switch' }
    if ($type -eq [bool]) { return 'bool' }
    if ($type.IsArray) { return 'list' }
    if (@([int], [long], [int16], [byte], [sbyte], [uint16], [uint32], [uint64]) -contains $type) { return 'int' }
    if (@([double], [single], [decimal]) -contains $type) { return 'number' }
    return 'string'
}

$help = $ast.GetHelpContent()
$paramHelp = @{}
if ($help) {
    foreach ($k in $help.Parameters.Keys) { $paramHelp[$k.ToUpperInvariant()] = $help.Parameters[$k].Trim() }
}

$params = @()
$sets = @{}
if ($ast.ParamBlock) {
    foreach ($p in $ast.ParamBlock.Parameters) {
        $name = $p.Name.VariablePath.UserPath
        $typeName = 'object'
        $type = $null
        foreach ($a in $p.Attributes) {
            if ($a -is [System.Management.Automation.Language.TypeConstraintAst]) {
                $typeName = $a.TypeName.Name
                $type = $a.TypeName.GetReflectionType()
            }
        }
        $o = [ordered]@{
            name     = $name
            kind     = Get-Kind $type
            type     = $typeName
            required = $false
            default  = ''
            choices  = @()
            help     = ''
            aliases  = @()
        }
        if ($p.DefaultValue) {
            $v = Get-Constant $p.DefaultValue
            $o.default = if ($v -is [bool]) { if ($v) { '$true' } else { '$false' } } else { [string]$v }
        }
        foreach ($a in $p.Attributes) {
            if ($a -isnot [System.Management.Automation.Language.AttributeAst]) { continue }
            switch -Regex ($a.TypeName.Name) {
                '^(System\.Management\.Automation\.)?Parameter(Attribute)?$' {
                    foreach ($na in $a.NamedArguments) {
                        switch ($na.ArgumentName) {
                            'Mandatory' { $o.required = $na.ExpressionOmitted -or [bool](Get-Constant $na.Argument) }
                            'HelpMessage' { $o.help = [string](Get-Constant $na.Argument) }
                            'ParameterSetName' { $sets[[string](Get-Constant $na.Argument)] = $true }
                        }
                    }
                }
                '^(System\.Management\.Automation\.)?ValidateSet(Attribute)?$' {
                    $o.choices = @($a.PositionalArguments | ForEach-Object { [string](Get-Constant $_) })
                }
                '^(System\.Management\.Automation\.)?Alias(Attribute)?$' {
                    $o.aliases = @($a.PositionalArguments | ForEach-Object { Get-Constant $_ } | ForEach-Object { [string]$_ })
                }
            }
        }
        if ($paramHelp.ContainsKey($name.ToUpperInvariant())) {
            $o.help = (@($paramHelp[$name.ToUpperInvariant()], $o.help) | Where-Object { $_ }) -join "`n"
        }
        $params += $o
    }
}

$notes = @()
if ($sets.Count -gt 1) { $notes += 'the script has parameter sets; each parameter is asked for regardless of set' }
if ($ast.DynamicParamBlock) { $notes += 'dynamic parameters are not listed' }

$summary = ''
if ($help) {
    $summary = if ($help.Synopsis) { $help.Synopsis.Trim() } elseif ($help.Description) { ($help.Description.Trim() -split "`n")[0].Trim() } else { '' }
}

$editions = @()
if ($ast.ScriptRequirements) { $editions = @($ast.ScriptRequirements.RequiredPSEditions) }

$result = [ordered]@{
    summary  = $summary
    params   = $params
    hasParam = $null -ne $ast.ParamBlock
    editions = $editions
    errors   = @($parseErrors | ForEach-Object { 'line {0}: {1}' -f $_.Extent.StartLineNumber, $_.Message })
    notes    = $notes
}
[Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes(($result | ConvertTo-Json -Depth 6 -Compress)))
