# pathed's PowerShell wrapper, printed by `pathed init pwsh`.
# pathed.exe changes the saved PATH, but a child process cannot touch this shell's $env:PATH.
# This compares the saved PATH before and after and applies the difference here, so entries
# only this session has (mise's, a venv's, ...) stay. A new entry goes to the end, whatever
# its saved position.
function pathed {
  # The saved PATH, Machine then User, one folder per item (with %VARS% expanded).
  $saved = {
    'Machine', 'User' |
      ForEach-Object { [Environment]::GetEnvironmentVariable('Path', $_) -split ';' } |
      Where-Object { $_ }
  }

  $before = & $saved
  pathed.exe @args
  $after = & $saved

  $removed = @($before | Where-Object { $_ -notin $after })
  $added = @($after | Where-Object { $_ -notin $before })
  $kept = @($env:PATH -split ';' | Where-Object { $_ -and $_ -notin $removed })
  $env:PATH = ($kept + @($added | Where-Object { $_ -notin $kept })) -join ';'
}
