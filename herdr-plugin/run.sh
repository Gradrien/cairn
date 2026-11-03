#!/bin/sh
# herdr runs plugin commands without a shell PATH: find cairn where `go install` puts it.
bin="${CAIRN_BIN:-}"
[ -z "$bin" ] && bin="$(command -v cairn 2>/dev/null)"
[ -z "$bin" ] && bin="$(go env GOBIN 2>/dev/null)/cairn"
[ -x "$bin" ] || bin="$HOME/go/bin/cairn"
exec "$bin" "$@"
