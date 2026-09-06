# d2

For docs, more installation options and the source code see https://github.com/d2lang/d2

version: v0.8.2
os: macos
arch: arm64

Built with go1.27.0.

## Install

```sh
make install DRY_RUN=1
# If it looks right, run:
# make install
```

Pass `PREFIX=somepath` to change the installation prefix from the default which is
`/usr/local` or `~/.local` if `/usr/local` is not writable with the current user.

## Uninstall

```sh
make uninstall DRY_RUN=1
# If it looks right, run:
# make uninstall
```
