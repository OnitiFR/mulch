#!/bin/bash

# go get github.com/vwochnik/gost

tmux \
    new-session 'cd test1 && gost -port 8081' \; \
    split-window 'cd test2 && gost -port 8082' \; \
    select-layout even-vertical
