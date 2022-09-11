#!/usr/bin/env bash

platforms=("windows/amd64" "darwin/amd64" "darwin/arm64" "linux/amd64")

for platform in "${platforms[@]}"
do
  platform_split=(${platform//\// })
  GOOS=${platform_split[0]}
  GOARCH=${platform_split[1]}
  GIT_TAG_NAME=$(git describe --tags --abbrev=0)

  output_name='wizfy'
  tar_name='wizfy-'$GOOS'-'$GOARCH'-'$GIT_TAG_NAME'.tar.gz'
  if [ $GOOS = "windows" ]; then
    output_name+='.exe'
  fi

  env GOOS=$GOOS GOARCH=$GOARCH go build -ldflags="-X 'wizfy/internal/build.Version=$GIT_TAG_NAME'" -o $output_name main.go
  tar -zcf $tar_name $output_name
  rm $output_name
  sha256sum $tar_name


  if [ $? -ne 0 ]; then
    echo 'An error has occurred! Aborting the script execution...\n'
    exit 1
  fi
done