# Run before the script when it has no console (the aex window): Read-Host asks aex instead, over the
# connection plugins use for their questions (internal/plugin/prompts.go), one JSON line each way.
# Where to connect is taken out of the environment, so processes the script starts do not get it.
$global:AexPrompts = @{ Addr = $env:AEX_PROMPTS; Token = $env:AEX_PROMPT_TOKEN }
Remove-Item Env:AEX_PROMPTS, Env:AEX_PROMPT_TOKEN -ErrorAction SilentlyContinue

function global:Read-Host {
    [CmdletBinding()]
    param(
        [Parameter(Position = 0, ValueFromRemainingArguments = $true)] $Prompt,
        [switch] $AsSecureString,
        [switch] $MaskInput
    )
    $p = $global:AexPrompts
    if (-not $p.Writer) {
        $host_, $port = $p.Addr -split ':'
        $p.Client = New-Object System.Net.Sockets.TcpClient($host_, [int]$port)
        $stream = $p.Client.GetStream()
        $utf8 = New-Object System.Text.UTF8Encoding($false)
        $p.Writer = New-Object System.IO.StreamWriter($stream, $utf8)
        $p.Reader = New-Object System.IO.StreamReader($stream, $utf8)
        $p.Writer.WriteLine($p.Token)
    }
    $title = (@($Prompt) | ForEach-Object { [string]$_ }) -join ' '
    if (-not $title) { $title = 'Answer' }
    $request = @{ kind = 'input'; title = $title; secret = [bool]($AsSecureString -or $MaskInput) }
    $p.Writer.WriteLine(($request | ConvertTo-Json -Compress))
    $p.Writer.Flush()
    $line = $p.Reader.ReadLine()
    if ($null -eq $line) { throw 'aex stopped answering questions' }
    $answer = $line | ConvertFrom-Json
    if ($answer.error) { throw "Read-Host: $($answer.error)" }
    $value = [string]$answer.value
    if ($AsSecureString) { return ConvertTo-SecureString $value -AsPlainText -Force }
    $value
}
