#!/bin/sh

set -ex

mkdir -p out/
go build -o out/app .

export IMAGE=registry.$BFC_DOMAIN/$BFC_USER/$BFC_APP:latest
docker build -t "$IMAGE" .
docker push "$IMAGE"
rm -rf out/
