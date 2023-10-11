#!/usr/bin/env bash

set -e

if [[ -f "$GITHUB_PAT_PATH" ]]; then
  echo "Sourcing Github pat from path"
  GITHUB_PAT="$(cat "$GITHUB_PAT_PATH")"
fi

if [[ "$GITHUB_PAT" != "" ]]; then
  echo "Rewriting to use Github pat"
  git config --global url."https://${GITHUB_PAT}:x-oauth-basic@github.com/".insteadOf "https://github.com/"
else
  echo "No Github pat given, rewriting to use plain ssh auth"
  git config --global url."git@github.com:".insteadOf "https://github.com"
fi

