# Incident report: the message hub deadlock

A single SSH client behind a half-dead TCP connection froze the SSH
proxy of a production mulchd for a few hours. This report details the
symptoms, the diagnosis, the root cause and the fix, because the bug
was subtle, six years old, and its investigation is a nice case study
of Go concurrency going wrong.

All host names, addresses and timestamps have been redacted or made
relative: only the mechanics matter here.

What happened?
--------------

Users could not SSH into any VM of one mulch server anymore. The
symptom was not "connection refused" but a silent hang:

```sh
$ ssh myvm@myserver
(nothing, then several minutes later…)
Connection timed out during banner exchange
```

Meanwhile, on the very same server:

- SSH to the host itself (port 22/…) worked normally.
- The mulchd HTTP API port was accepting TLS connections.
- `mulch-proxy` was serving all hosted web applications normally.
- systemd reported `mulchd.service` as `active (running)`, with a
  modest memory footprint.

So everything *looked* fine, except the mulch SSH proxy.

#### The first clue: TCP accepts, SSH never answers

The SSH proxy port accepted TCP connections (the three-way handshake
completed) but never sent the SSH banner. That combination is
interesting: the TCP handshake is completed by the *kernel*, using the
listen backlog, without any cooperation from the application. So a
port can look "open" while the process behind it never calls
`accept()` again.

And indeed:

```sh
$ ss -tln 'sport = :<ssh-proxy-port>'
State  Recv-Q Send-Q
LISTEN 70     4096
```

70 connections were parked in the accept queue. Nobody was picking
them up. (For the record: this is *not* an attack signature — a few
dozen queued connections is just a few hours of retries and routine
Internet background noise once the accept loop stops draining.)

#### The second clue: the daemon had gone silent

The journal told the rest of the story. mulchd's own log messages
(the `INFO(…)`/`SUCCESS(…)` lines) stopped at some point during the
night — let's call it **T+0** — right at the end of a session where a
user was rebuilding VMs and streaming build logs with the `mulch`
client. After T+0, the only lines still appearing were raw
`http: TLS handshake error` messages from Go's `net/http` package.

That contrast is the smoking gun: `net/http` writes its errors
directly with the standard `log` package, but every mulchd log line
goes through the **message hub**. HTTP was alive, the hub was dead.

At **T+5h40**, mulchd was restarted and everything came back
instantly. The 70 queued connections were, of course, lost to a long
`banner exchange` timeout, but new ones worked fine.

The root cause
--------------

The message hub (`cmd/mulchd/server/hub.go`) is a small
publish/subscribe loop: producers call `Log.*()` which ends up in
`Hub.Broadcast()`, and consumers (mainly `mulch` CLI clients streaming
logs over HTTP) each get a `Messages` channel, served by the single
`Hub.Run()` goroutine.

Three innocent-looking lines chained into a full freeze:

#### 1. The HTTP stream writer had no deadline

`routeStreamHandler()` (`cmd/mulchd/server/route_handler.go`) streamed
messages with a plain `enc.Encode(msg)` on the response writer, and
the HTTP servers set no `WriteTimeout` (on purpose: these are
long-lived streams). If the client's TCP connection was half-dead —
a laptop closed mid-stream, a NAT mapping silently expired — the
write blocked forever once the kernel send buffer filled up. Worse:
even when `Encode` *did* return an error, the handler just printed it
and kept looping instead of unregistering the client.

#### 2. The hub used an unguarded blocking send

`Hub.Run()` delivered each message with:

```go
client.Messages <- message
```

An unbuffered, blocking send. The handler above was stuck in a write
and no longer reading `Messages`, so `Hub.Run()` blocked on this line.
Forever. Since `Run()` is a single goroutine serving `broadcast`,
`register` *and* `unregister`, the whole hub was now inert: every
subsequent `Log.*()` call in the entire daemon blocked on
`hub.broadcast`.

#### 3. The SSH proxy accept loop logs

The accept loop of the SSH proxy (`ListenAndServeProxy()` in
`cmd/mulchd/server/ssh_proxy.go`) calls `app.Log.Tracef()` right after
`Accept()`. On the first connection after T+0, that `Tracef` blocked
on the dead hub, and `Accept()` was never called again. The kernel
kept completing handshakes into the backlog — hence "TCP accepts, SSH
never answers", and the Recv-Q of 70.

So the full chain is: dead TCP peer → blocked stream write → hub
goroutine blocked → all logging blocked → SSH accept loop blocked.
One remote client, connected read-only to a log stream, took down the
SSH service of the whole host. May the sysadmin gods forgive us.

#### A second deadlock, hiding in the same lines

There is an even simpler variant, needing no dead TCP connection at
all. When a streaming client disconnects, the handler stops reading
`Messages` and calls `client.Unregister()`, which sends on the
`hub.unregister` channel. If `Hub.Run()` is *at that moment* blocked
sending to this very client's `Messages` channel, both sides wait for
each other: a textbook mutual block. This matches an old TODO.txt
entry about a mulchd "outage" where the freeze followed "a command
break from a user" (an interrupted `mulch` command, i.e. a vanishing
stream consumer). That entry dated back to 2020 and already asked:
"check for a possible deadlock / missing timeout in the message
hub?". Yes. It was that.

#### Red herrings

- The memory peak reported by systemd was huge compared to the
  resident size at freeze time, and looked like the villain. It was
  unrelated (large VM image operations earlier that day); the
  deadlock accumulates nothing since producers block instead of
  queueing.
- The parked connections in the accept queue looked like a SYN flood.
  They were the *consequence* of the freeze, not its cause.

The fix
-------

Three surgical changes, defense in depth:

#### hub.go: bounded sends

`Hub.Run()` now delivers with a timeout, dropping the message (with a
warning on stdout — we obviously can't use the hub to log its own
demise) if a client doesn't read in time. Note that we deliberately do
*not* unregister the slow client inline: an earlier version of the hub
did exactly that from a `default:` arm and was removed years ago for
race conditions (closing a channel that consumers still `range` over).
`Run()` remains the only closer of `Messages`, through the normal
unregister path. A slow-but-alive client just loses some messages.

This single change bounds both deadlock variants: the hub can no
longer be held hostage by any consumer.

#### route_handler.go: write deadlines on the stream

Each write on the message stream (messages *and* keep-alives) now sets
a write deadline via `http.ResponseController`, and a write error now
unregisters the client and ends the handler instead of being printed
and ignored. A dead client is therefore detected and evicted in at
most ~20 seconds (next keep-alive + deadline) instead of holding a hub
slot forever.

#### ssh_proxy.go: a more paranoid accept loop

Two latent bugs in the accept loop were fixed while we were there:
the remote address was logged *before* the `Accept()` error check (a
guaranteed nil dereference on any accept error, e.g. running out of
file descriptors), and any accept error terminated the loop — and the
listener — permanently. Errors other than `net.ErrClosed` are now
logged and retried after a short pause.

#### Tests

`hub_test.go` adds two stdlib-only regression tests replaying both
deadlock shapes: a client that never reads must not block `Broadcast`
for others, and an `Unregister` racing a blocked send must not
deadlock the hub. Both fail (by timeout) on the pre-fix code.

If it ever happens again…
-------------------------

A checklist for the next silent freeze, whatever its cause:

- Compare `mulchd`-formatted log lines with raw `net/http` ones in the
  journal: "HTTP noise continues, mulchd lines stopped" means the hub
  (or something upstream of it) is stuck, not the process.
- `ss -tln` on the SSH proxy port: a non-zero `Recv-Q` on a LISTEN
  socket means the accept loop is not draining.
- **Before** restarting, capture the goroutine stacks: `kill -QUIT`
  the process (stacks go to stderr, i.e. the journal), or use the
  pprof goroutine profile. The blocked `Hub.Run()` frame is
  unmistakable there — this report would have taken an hour instead
  of an evening with a stack dump of the frozen process.
