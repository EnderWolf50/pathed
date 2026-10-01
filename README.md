# pathed

**pathed now lives in [enved](https://github.com/EnderWolf50/enved).** This repository is
archived; v0.5.0 is its last release.

The `pathed` command is unchanged: the same editor of the User and Machine PATHs, the same
`list`, `add`, `rm`, `clean` and `init pwsh`. It is built from enved's repository, which
also offers it as `enved path`:

```sh
go install github.com/EnderWolf50/enved/cmd/pathed@latest
```

or download `pathed.exe` from [enved's releases](https://github.com/EnderWolf50/enved/releases/latest).

Its settings are now enved's: `~/.config/enved/config.toml` (or `$ENVED_CONFIG`); copy your
`~/.config/pathed/config.toml` there if you changed it.

## License

MIT
