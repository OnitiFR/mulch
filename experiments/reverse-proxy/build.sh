#!/bin/bash

go build && scp reverse-proxy cobaye1:tmp && rm -f reverse-proxy

