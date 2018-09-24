#!/bin/bash

# Installer tmux
# CTRL+c à répétition (3 fois donc) pour sortir

tmux \
    new-session 'cd test1 && http-server -p 8081' \; \
    split-window 'cd test2 && http-server -p 8082' \; \
    select-layout even-vertical
