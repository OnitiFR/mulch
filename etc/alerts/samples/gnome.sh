#!/bin/bash

# Send mulchd alerts as Gnome/freedesktop desktop notifications (libnotify).
# Requires mulchd to run inside the graphical session.

urgency="critical"
icon="dialog-error"
expire="0" # critical alerts stay on screen
if [ "$TYPE" = "GOOD" ]; then
    urgency="normal"
    icon="emblem-default"
    expire="10000"
fi

notify-send -a mulch -u "$urgency" -i "$icon" -t "$expire" \
    "[$TYPE] $(hostname -s): $SUBJECT" "$CONTENT"
