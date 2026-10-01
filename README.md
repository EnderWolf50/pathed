# pathed

View and edit the persistent Windows `PATH`, User and Machine, from a TUI or the command
line.

Windows only: there the `PATH` is a setting the system keeps (in the registry). On Linux and
macOS it is put together at login by shell startup files (`~/.profile`, `~/.zshrc`,
`/etc/paths.d`, ...), so the place to change it is those files.

It writes the registry directly, so `REG_EXPAND_SZ` values and their `%VARS%` stay intact
(.NET's `SetEnvironmentVariable` rewrites them as `REG_SZ`). Every write first saves the old
value to `%LOCALAPPDATA%\enved\` (older versions used `%LOCALAPPDATA%\pathed\`), refuses to
overwrite a PATH something else changed since pathed read it, and tells running programs the
environment changed, so new windows see it without signing out.

pathed is the PATH-only sibling of [enved](https://github.com/EnderWolf50/enved), which edits
every environment variable and opens `Path` (and other lists) in this same editor. The
registry, UAC and screen code live in enved's packages; pathed is a thin program over them.

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
pathed init pwsh         print a PowerShell wrapper that also updates the current shell
```

`-m` works on the Machine `PATH`. Writing it needs admin: unless pathed already runs
elevated, saving asks UAC, and an elevated copy of pathed does the write.

In the editor the sidebar holds the two PATHs (`*` unsaved changes, `uac` saving it will ask
for admin, `!` could not be read or saved); `enter` opens one. Changes are only marked until you save:
added entries show green, edited ones amber, removed ones red and struck through.

| Key | In the list |
| --- | --- |
| `a` / `i` | add a folder after / before the cursor (checked as you type: exists? already listed?) |
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

pathed changes the saved `PATH`, but the shell it runs in keeps its own copy, which a program
the shell starts cannot change. `pathed init pwsh` prints a PowerShell function named
`pathed` that runs `pathed.exe` and then applies the change to the shell as well: entries
that were added go to the end of `$env:PATH`, removed ones are dropped, and entries only this
session has (from mise, a venv, ...) stay. To load it in every shell, add this to your
`$PROFILE`:

```powershell
pathed.exe init pwsh | Out-String | Invoke-Expression
```

## License

MIT
