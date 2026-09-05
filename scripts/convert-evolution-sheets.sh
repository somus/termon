#!/usr/bin/env bash
set -euo pipefail

output_dir=${1:-docs/design/monster-sprites}

./scripts/convert-contact-sheet.sh \
  docs/design/evolution-sheets/sheet-a-organic-thermal.png "$output_dir" 32 30 \
  barkdoor,vinemount,briarwall,lichenloop,taprouter,cinderhash,priviloak,canopynet,fortiforest,bogdaemon,rhizoracle,flarestack

./scripts/convert-contact-sheet.sh \
  docs/design/evolution-sheets/sheet-b-thermal-coolant.png "$output_dir" 32 30 \
  furnacehub,burnboard,torchthread,bytefin,wavebank,pipelinx,calderdaemon,infernalink,daemoflare,datadeluge,tidalarray,torrentiger

./scripts/convert-contact-sheet.sh \
  docs/design/evolution-sheets/sheet-c-coolant-current.png "$output_dir" 32 30 \
  fogbuffer,cachelotl,voltalon,voltweiler,coilobra,stormfin,cloudvault,rebootide,stormkernel,ampmastiff,gridaconda,tempestray

./scripts/convert-contact-sheet.sh \
  docs/design/evolution-sheets/sheet-d-virus-silicon.png "$output_dir" 32 30 \
  mailgnant,featurmoil,segmaggot,solderat,trackbyte,ramhog,phishmonger,heapocalypse,hexhelminth,rackoon,watchdaemon,racktusk

echo "wrote 48 evolution sprites to $output_dir"
