#!/bin/bash -e

cleanup() {
    CODE=$?
    rm -rf $TEMP_DIR
    exit $CODE
}

trap cleanup EXIT

TEMP_DIR="$( mktemp -d )"

SHA=$( git rev-parse HEAD )

REPO=$TEMP_DIR/ericchiang.github.io
git clone git@github.com:ericchiang/ericchiang.github.io.git $REPO
rm -r $REPO/*
ls -al $REPO

if [ -d public ]; then
    rm -r public
fi

go run github.com/gohugoio/hugo
rsync -av public/* $REPO/
cd $REPO
git add .
git commit -m "built from $SHA"

git push git@github.com:ericchiang/ericchiang.github.io.git
