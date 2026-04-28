#!/usr/bin/env sh
case "$npm_config_user_agent" in
  bun/*)
    exit 0
    ;;
  *)
    echo ""
    echo "This project uses bun. Install with:"
    echo "  \$ bun install"
    echo ""
    exit 1
    ;;
esac
