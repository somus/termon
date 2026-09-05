#!/usr/bin/env bash
set -euo pipefail

input=${1:-docs/design/monster-roster-source.png}
sprite_width=${3:-32}
sprite_height=${4:-30}
output_dir=${2:-docs/design/monster-sprites}
names=${5:-rootkit,sproutware,thornpatch,mossmuff,rootanami,emberbyte,cindernode,scorchip,wickware,aquabit,flowcell,gushkit,mistcache,splashscreen,zaplet,joulpup,amperent,surgetail,spamlet,bloatware,wormate,chippunk,coghound,servoboar}

if ! command -v magick >/dev/null 2>&1; then
  echo "ImageMagick 7 is required (missing: magick)" >&2
  exit 1
fi

if [[ ! -f "$input" ]]; then
  echo "contact sheet not found: $input" >&2
  exit 1
fi

mkdir -p "$output_dir"
raw_dir=$(mktemp -d)
trap 'rm -rf -- "$raw_dir"' EXIT

go run ./cmd/sheetextract --names "$names" "$input" "$raw_dir"

count=0
for raw_sprite in "$raw_dir"/*.png; do
  filename=${raw_sprite##*/}
  magick "$raw_sprite" \
    -filter box -resize "${sprite_width}x${sprite_height}>" \
    -colors 24 -dither None \
    -channel A -threshold 30% +channel \
    -gravity south -background none -extent "${sprite_width}x${sprite_height}" \
    "$output_dir/$filename"
  count=$((count + 1))
done

echo "wrote $count sprites to $output_dir"
