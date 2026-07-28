#!/bin/sh
set -eu

sqlc generate
git diff --exit-code -- internal/postgres/sqlcgen
