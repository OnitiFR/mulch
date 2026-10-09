# Mulch client guide for AI agents / LLMs

This guide is meant for AI agents driving the `mulch` CLI on behalf of a human.
Read it entirely before running other commands. A generated command reference
is appended at the end.

This guide does not document each command. Before using a command for the
first time, run `mulch <command> --help`: it gives syntax, examples and
warnings (ex: `mulch vm search --help` for the query syntax).

## What is Mulch?

Mulch is a light virtual machine manager (KVM/libvirt), a kind of "hardened
container system". `mulch` is the client: it talks to one or more `mulchd`
servers using a REST API. The server does all the work and streams its logs
back to the client.

## Server selection

- The client may know several servers. List them with
  `mulch --dump-servers` (add `-t` to see aliases).
- The default server is used unless `-s <server>` is given. `-s` also
  accepts server aliases.
- **Do not read the client configuration file**: it contains API keys, and
  you don't need it.
- When several servers exist and the human did not name one:
  - read-only commands: use the default server, and say which one in your
    answer;
  - any command that changes something: ask which server first.

## Core concepts

- **VM description file**: a TOML file (name, seed, disk/ram/cpu, domains,
  scripts, env, secrets, …). Get the one of an existing VM with
  `mulch vm config <vm>`.
- **Seed**: base Linux image a VM is created from (`mulch seed list`).
- **Lifecycle scripts**: shell scripts run as `admin` (sudoer) or `app`
  (application user): *prepare*, *install* (new VM), *backup*, *restore*.
- **Revisions**: a VM is identified by name *and* revision (`myvm-r0`,
  `myvm-r1`, …). Only one revision is **active** (it receives HTTP traffic
  and is the default target of every command). Use `-r <revision>` to target
  another one.
- **Lock**: a locked VM can't be deleted, and most disruptive actions require
  `--force` on it. Locks exist to protect important VMs.
- **Backups**: qcow2 disk images filled by backup scripts, used for restore,
  rebuild, and migration.
- **Rebuild**: recreate a VM from a fresh seed using a transient backup of
  itself. Relies on backup/restore scripts correctness.
- **Secrets**: server-side encrypted values, injected as environment
  variables in VMs that list them.
- **Do-actions**: per-VM custom scripts, listed with `mulch do <vm>`.

## Safety rules (important)

Mulch has **no confirmation prompts**: every command executes immediately.

Always ask the human for explicit confirmation before running:

- Data loss: `vm delete`, `vm rebuild`, `vm redefine`, `backup delete`,
  `secret delete`, `secret set` (overwrites).
- Downtime or traffic changes: `vm stop`, `vm restart`, `vm activate`,
  `vm deactivate`, `vm migrate`, `vm create` without `-i` when a revision
  is already active.
- Access rights: `key create`, `key delete`, `key right add/remove/clear`,
  `trust forward`, `trust remove`.
- Any use of `--force` (it bypasses a lock, which is a deliberate safety).

Also:

- `mulch secret get` prints secret values: avoid it unless the human asks,
  and never repeat secret values back.
- Prefer read-only commands to investigate: `vm list`, `vm infos`,
  `vm config`, `vm search`, `backup list`, `seed list`, `log`, `status`.

## Output and exit codes

- Output is human-oriented. Use `-b` / `--basic` on list commands (`vm list`,
  `backup list`, `do <vm>`, …) to get simple, parsable output. `status`
  supports `--json`.
- Long operations (create, rebuild, backup, …) stream server logs until a
  final SUCCESS or FAILURE message. The exit code is non-zero on failure.
- Interrupting the client does **not** cancel a server operation. Use
  `mulch log` (or `mulch log <vm>`) to see what happened, and `vm abort`
  only if the human asks.

## Commands to avoid or adapt (interactive / never-ending)

- `mulch ssh <vm>` without a command opens an interactive shell. Instead,
  run a command: `mulch ssh <vm> -- <command>` (`-a` for admin user).
  Stdin is forwarded: `mulch ssh <vm> -- 'cat > f.txt' < local.txt`.
- `mulch log -f` and `mulch vm console` never end by themselves.
- `mulch secret set <name>` reads the value from stdin: pipe it.
- `mulch backup mount` requires guestmount and a local mount point.

## Common recipes

```sh
mulch vm list -b                          # VM names
mulch vm infos myvm                       # IP, resources, state, …
mulch vm config myvm > myvm.toml          # get description file
mulch ssh myvm -- tail -n 50 /var/log/syslog
mulch vm exec myvm admin script.sh        # run a local script in the VM
mulch vm backup myvm                      # create a backup
mulch backup list -b

# blue/green deployment (ask before activating!)
mulch vm create -n -i myvm.toml           # new inactive revision
mulch vm activate myvm <revision>         # switch traffic
```

Use `mulch <command> --help` (or `mulch help <command>`) for full details
on any command.
