#!/bin/sh

tr ',' '\n' | while read n; do
    printf "%b" "$(printf '\\x%02x' "$n")"
done > out.bin
