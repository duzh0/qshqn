#!/usr/bin/env bash
set -e

git ls-files -co --exclude-standard \
  | grep -Ev 'init\.json|\.(db|session|ico|png|gif|woff2|db-shm|db-wal)$|dump\.txt|facefinder|predator_msgs\.json|(daily|monthly)\.json' \
  | while read -r file; do
      echo -e "\n### File: $file"
      echo '```'
      awk 'NF' "$file"
      echo '```'
    done > out/dump.txt
