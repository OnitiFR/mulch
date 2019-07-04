#!/bin/bash

hook_url="https://hooks.slack.com/services/xxx"

mark=":exclamation:"
if [ "$TYPE" = "GOOD" ]; then
    mark=":heavy_check_mark:"
fi

payload=$(cat <<EOT
{
    "text": "$mark [$TYPE] $(hostname -s) : $SUBJECT - $CONTENT"
}
EOT
)

curl -s -f -w "\nHTTP Code %{http_code}\n" -X POST --data-urlencode "payload=$payload" "$hook_url"
if [ $? -ne 0 ]; then
    exit 1
fi
