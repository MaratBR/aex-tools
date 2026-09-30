#Requires AutoHotkey v2.0
#SingleInstance Force

; Echoes its args.
; One per line, then exits with code 3 when the first is "fail".

; not part of the summary
out := A_Args.Length ":"
for a in A_Args
    out .= "|" a
FileAppend out "`n", "*", "UTF-8"
if A_Args.Length && A_Args[1] = "fail"
    ExitApp 3
ExitApp
