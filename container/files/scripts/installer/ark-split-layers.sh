#!/usr/bin/env bash

# Spread the ARK server install over <dest>/0..<bins> so the final image can COPY
# each directory as its own layer. As one layer the ~11 GB of pak files sits at
# GHCR's 10 GB per-layer limit and pushes/pulls as a single fragile blob.
# <dest>/0 gets everything except the pak files; <dest>/1..<bins> share the pak
# files, largest first into the currently smallest directory.
set -euo pipefail

src="$1"
dest="$2"
bins="$3"
paks="ShooterGame/Content/Paks"

declare -a size
for ((i = 1; i <= bins; i++)); do
  mkdir -p "$dest/$i/$paks"
  size[i]=0
done

while read -r bytes name; do
  min=1
  for ((i = 2; i <= bins; i++)); do
    if ((size[i] < size[min])); then min=$i; fi
  done
  mv "$src/$paks/$name" "$dest/$min/$paks/"
  size[min]=$((size[min] + bytes))
done < <(find "$src/$paks" -maxdepth 1 -type f -printf '%s %f\n' | sort -rn)

mv "$src" "$dest/0"

for ((i = 1; i <= bins; i++)); do
  echo "layer $i: $((size[i] / 1024 / 1024)) MiB of pak files"
done
