# pathed

View and edit the persistent Windows `PATH`, User and Machine, from a TUI or the command
line.

It writes the registry directly, so `REG_EXPAND_SZ` values and their `%VARS%` stay intact
(.NET's `SetEnvironmentVariable` rewrites them as `REG_SZ`). Every write first saves the old
value to `%LOCALAPPDATA%\pathed\`, and running programs are told the environment changed,
so new windows see it without signing out.

## Install

Download `pathed.exe` from the [latest release](https://github.com/EnderWolf50/pathed/releases/latest)
and put it on your `PATH`, or build it with Go 1.27+:

```sh
go install github.com/EnderWolf50/pathed@latest
```

## Use

```
pathed                   interactive editor (User and Machine)
pathed list [-m]         numbered entries; missing folders and duplicates are marked
pathed add <dir> [-m] [--front]
pathed rm <dir|N> [-m]   N is the number shown by 'pathed list'
pathed clean [-m]        drop duplicates and folders that do not exist
```

`-m` works on the Machine `PATH`, which needs an elevated shell (`gsudo pathed ...`).

In the editor: `↑`/`↓` move, `K`/`J` reorder, `a` add, `e` edit, `d` delete, `c` clean,
`u` revert, `s` save, `tab` switch User/Machine, `q` quit.

pathed changes the saved `PATH`; the shell it runs in keeps its own copy. To update that
too, wrap it, e.g. in PowerShell:

```powershell
function pathed {
  $saved = { ('Machine', 'User' | ForEach-Object { [Environment]::GetEnvironmentVariable('Path', $_) }) -join ';' -split ';' | Where-Object { $_ } }
  $before = & $saved
  pathed.exe @args
  $after = & $saved
  $session = @($env:PATH -split ';' | Where-Object { $_ -and ($_ -in $after -or $_ -notin $before) })
  $env:PATH = ($session + @($after | Where-Object { $_ -notin $before -and $_ -notin $session })) -join ';'
}
```

## License

MIT
