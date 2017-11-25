#!/bin/bash -e

TEMPDIR=$(mktemp -d)

cleanup() {
    CODE=$?
    rm -rf $TEMPDIR
    exit $CODE
}

trap cleanup EXIT

SHA=$( git rev-parse HEAD )

REPO=$TEMPDIR/ericchiang.github.io
git clone https://github.com/ericchiang/ericchiang.github.io $REPO
rm -r $REPO/*
ls -al $REPO

if [ -d public ]; then
    rm -r public
fi

hugo
cp -r public/* $REPO
ls -al $REPO
cd $REPO
git add .
git commit -m "built from $SHA"

git push https://$GITHUB_USERNAME:$GITHUB_PASSWORD@github.com/ericchiang/ericchiang.github.io
