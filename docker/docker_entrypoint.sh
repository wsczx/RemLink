#!/bin/bash
var1=$1

case $var1 in
"bash" | "sh")
  echo $var1
  exec "$@"
  ;;

"tool")
  /app/remlink "$@"
  ;;

*)
  iptables -V
  exec /app/remlink "$@"
  ;;
esac
