#!/bin/bash

go build && scp https-test cobaye1:tmp && rm -f https-test
