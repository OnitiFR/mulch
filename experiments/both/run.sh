#!/bin/bash

go build && scp both cobaye1:tmp && rm -f both && \
 ssh cobaye1 sudo setcap 'cap_net_bind_service=+ep' tmp/both && \
 ssh cobaye1 "cd tmp ; ./both"
