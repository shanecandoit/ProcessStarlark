#!/bin/sh
set -eu
cd "$(dirname "$0")"

sh web/build.sh
python3 -m http.server -d web/dist 8000
