#!/usr/bin/env bash

args="${@}"

npm=$(command -v npm)
workDir=$(pwd)

publish() {
  local dir=$1
  log "processing $dir"

  if [ -d "$workDir/output/$dir" ]; then
    cp "$workDir/output/index.json" "$workDir/output/$dir/"
    cd "$workDir/output/$dir" || return
    $npm publish --access public
  else
    log "$workDir/output/$dir not exists"
  fi
}

log() {
  echo "[publish] $1"
}

log "pwd is $workDir"

declare -a arr=("op.gg" "op.gg-aram" "murderbridge" "lolalytics" "lolalytics-aram")

./data-crawler $args

cd "$workDir" || return

for i in "${arr[@]}"; do
  publish "$i"
done
