#!/usr/bin/env bash

set -e

BASEDIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )

for dir in "$BASEDIR"/../config/*
do
  [[ -e "$dir" ]] || break
  [[ "$dir" != *"config/samples"* ]] || break
  echo "TODO: fix kustomize checks"
#  TODO: fix later
#  kustomize build "$dir" > /dev/null
done
