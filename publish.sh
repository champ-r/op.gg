#!/usr/bin/env bash

args="${@}"

npm=$(command -v npm)
workDir=$(pwd)

declare -a arr=("op.gg" "op.gg-aram" "murderbridge" "lolalytics" "lolalytics-aram")

./data-crawler $args

publish() {
  local dir=$0
  echo "processing $dir"

  if [ -d "$workDir/output/$dir" ]; then
    cp "$workDir/output/index.json" "$workDir/output/$dir/"
    cd "$workDir/output/$dir" || return
    $npm publish --access public
  else
    echo "$workDir/output/$dir not exists"
  fi
}

for i in "${arr[@]}"; do
  publish "$i"
done
