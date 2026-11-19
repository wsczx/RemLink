#!/bin/bash

case $1 in
"bash" | "sh")
  exec "$@"
  ;;

"tool")
  exec /app/remlink "$@"
  ;;

*)
  iptables -V
  exec /app/remlink "$@"
  ;;
esac
