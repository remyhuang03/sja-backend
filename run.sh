#!/usr/bin/env sh
set -eu
# Environment is supplied by the caller; never print credentials.
exec go run .
