#!/bin/bash

# Alert parameters are stored in environment:
# TYPE ("GOOD" / "BAD" / "SECURITY")
# SUBJECT
# CONTENT
#
# WARNING: SUBJECT and CONTENT are NOT trusted. They are single-line (no
# control characters), but may contain anything else (quotes, $, backslashes,
# <, &, …), including text chosen by users (key comments, SSH key comments…).
# Always escape them for your output format (JSON, HTML, SQL, …), and never
# evaluate them (eval, sh -c, unquoted expansion, …).

hook_url="https://hooks.slack.com/services/xxx"

mark=":exclamation:"
if [ "$TYPE" = "GOOD" ]; then
    mark=":heavy_check_mark:"
elif [ "$TYPE" = "SECURITY" ]; then
    mark=":lock:"
fi

# escape a string for Slack (&, <, >: no mentions or links injection),
# then for a JSON string (backslash, double quote)
escape() {
    local s=$1
    s=${s//"&"/"&amp;"}
    s=${s//"<"/"&lt;"}
    s=${s//">"/"&gt;"}
    s=${s//"\\"/"\\\\"}
    s=${s//"\""/"\\\""}
    printf '%s' "$s"
}

subject=$(escape "$SUBJECT")
content=$(escape "$CONTENT")

payload=$(cat <<EOT
{
    "text": "$mark [$TYPE] $(hostname -s) : *$subject* - $content"
}
EOT
)

curl -s -f -w "\nHTTP Code %{http_code}\n" -X POST --data-urlencode "payload=$payload" "$hook_url"
if [ $? -ne 0 ]; then
    exit 1
fi
