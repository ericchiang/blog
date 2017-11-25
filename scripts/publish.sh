#!/bin/bash -e

cleanup() {
    CODE=$?
    rm -rf $HOME/ericchiang.github.io
    exit $CODE
}

trap cleanup EXIT

SHA=$( git rev-parse HEAD )

REPO=$HOME/ericchiang.github.io
git clone https://$GITHUB_USERNAME:$GITHUB_PASSWORD@github.com/ericchiang/ericchiang.github.io.git $REPO
rm -r $REPO/*
ls -al $REPO

if [ -d public ]; then
    rm -r public
fi

hugo version
hugo
rsync -av public/* $REPO/
cd $REPO
git add .
git commit -m "built from $SHA"

git push https://$GITHUB_USERNAME:$GITHUB_PASSWORD@github.com/ericchiang/ericchiang.github.io.git
