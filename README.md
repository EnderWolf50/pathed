# pathed

View and edit the persistent Windows `PATH`, User and Machine, from a TUI or the command
line.

Windows only: there the `PATH` is a setting the system keeps (in the registry). On Linux and
macOS it is put together at login by shell startup files (`~/.profile`, `~/.zshrc`,
`/etc/paths.d`, ...), so the place to change it is those files.

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

`-m` works on the Machine `PATH`. Writing it needs admin: unless pathed already runs
elevated, saving asks UAC, and an elevated copy of pathed does the write.

In the editor the sidebar holds the two PATHs (`*` unsaved changes, `uac` saving it will ask
for admin, `!` could not be read or saved); `enter` opens one. Changes are only marked until you save:
added entries show green, edited ones amber, removed ones red and struck through.

| Key | In the list |
| --- | --- |
| `a` | add a folder after the cursor (checked as you type: exists? already listed?) |
| `enter`, `e` | edit the entry |
| `d` | remove the entry, or keep it again |
| `K` / `J` | move the entry up / down |
| `c` | mark every missing folder and duplicate for removal |
| `u` | undo every change to this PATH |
| `o` | open the folder in Explorer |
| `/` | filter |
| `r` | read the PATH again |
| `s` | review the changes, then save them |
| `←` `h` `esc` `q` | back to the sidebar |

Settings (theme colors, sidebar width) live in `~/.config/pathed/config.toml`, or the file
named by `$PATHED_CONFIG`; `pathed --default-config` prints a commented starting point.

pathed changes the saved `PATH`; the shell it runs in keeps its own copy. To update that
too, wrap it, e.g. in PowerShell:

```powershell
# The saved PATH, Machine then User, one folder per item.
function Get-SavedPath {
  'Machine', 'User' |
    ForEach-Object { [Environment]::GetEnvironmentVariable('Path', $_) -split ';' } |
    Where-Object { $_ }
}

function pathed {
  $before = Get-SavedPath
  pathed.exe @args
  $after = Get-SavedPath

  $removed = @($before | Where-Object { $_ -notin $after })
  $added = @($after | Where-Object { $_ -notin $before })
  $kept = @($env:PATH -split ';' | Where-Object { $_ -and $_ -notin $removed })
  $env:PATH = ($kept + @($added | Where-Object { $_ -notin $kept })) -join ';'
}
```

## License

MIT
