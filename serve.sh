
set -e

web/build.sh
python3 -m http.server -d web/dist 8000
