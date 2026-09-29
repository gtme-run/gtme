#!/bin/sh
# Prints docs_only=true to $GITHUB_OUTPUT when this is a pull request whose
# changes are all under docs/. Anything else (a push, an empty diff, a file
# outside docs/) is not docs-only and gets the full suite.
event=$1 base=$2
docs_only=false
if [ "$event" = pull_request ] && [ -n "$base" ]; then
  files=$(git diff --name-only "$base" HEAD)
  if [ -n "$files" ] && ! printf '%s\n' "$files" | grep -qv '^docs/'; then
    docs_only=true
  fi
  printf 'changed files:\n%s\n' "$files"
fi
echo "docs_only=$docs_only"
echo "docs_only=$docs_only" >> "${GITHUB_OUTPUT:-/dev/null}"
