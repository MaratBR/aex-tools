<#
.SYNOPSIS
Greets someone.
.PARAMETER Name
Who to greet.
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory, HelpMessage = 'first name')][Alias('n')][string]$Name,
    [ValidateSet('hi', 'hello')][string]$Word = 'hi',
    [int]$Count = 1,
    [switch]$Loud,
    [bool]$Polite = $true,
    [string[]]$Items,
    [Parameter(Mandatory = $false)][double]$Ratio
)
"Name=$Name|Word=$Word|Count=$Count|Loud=$Loud|Polite=$Polite|Items=$($Items.Count):$($Items -join '+')|Ratio=$Ratio"
if ($Name -eq 'fail') { exit 3 }
