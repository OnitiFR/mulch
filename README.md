# Mulch
Easy VM creation and management tool: velvet applications in an iron hypervisor

What is it?
---
Mulch is a light and practical virtual machine manager, it allows to create and host applications in VMs with
a single command line and a simple description file. **You can see it as a simple KVM hardened container system**.

Index
---
 - [What's the point?](#whats-the-point)
 - [Show me!](#show-me)
 - [Any more complete example?](#any-more-complete-example)
 - [How does it works exactly?](#how-does-it-works-exactly)
 - [VM revisions](#vm-revisions)
 - [Show me more features!](#show-me-more-features)
 - [How do I install the client?](#how-do-i-install-the-client)
 - [How do I install the server?](#how-do-i-install-the-server-mulchd-and-mulch-proxy)

What's the point?
---
It aims people and organizations who don't want to (or can't) invest too much time in creating their own
private cloud, but want a simple solution with great performance.

It features a client-server architecture using a REST API. It's written in Go and use shell scripts to drive VM
configuration and application installations.

Mulch relies on libvirt API (KVM) and can share an existing libvirt server or use a dedicated one. No libvirt
configuration (or knowledge) is required, Mulch will create and manage needed resources (storage, network).
You will only interact with Mulch.

![Tech banner](https://raw.github.com/OnitiFR/mulch/master/doc/images/tech-banner.png)

Base Linux images ("seeds") use Cloud-Init so almost every [OpenStack compliant image](https://docs.openstack.org/image-guide/obtain-images.html) will work out of the box. Default Mulch
configuration provides seeds for Debian, Ubuntu and CentOS. Seed images will be downloaded and updated
automatically, it is Mulch's job, not yours.

Mulch features an embedded high-performance HTTP / HTTP2 reverse proxy with automatic Let's Encrypt certificate
generation for VM hosted Web Applications.

Mulch also provides an SSH proxy providing full access (ssh, scp, port forwarding, …) to VMs for Mulch users.

Show me!
---
Here's a minimal VM description file: (TOML format)
```toml
# This is a (working) minimalist sample VM definition
name = "mini"
seed = "debian_12"

disk_size = "20G"
ram_size = "1G"
cpu_count = 1
```
You can then create this VM with:
```sh
mulch vm create mini.toml
```

![mulch vm create minimal](https://raw.github.com/OnitiFR/mulch/master/doc/images/mulch-create-mini.png)

In this example, the VM is up and ready 40 seconds later.

Any more complete example?
---
A production VM configuration file may look like this (WordPress website):
```toml
name = "xfennec"

seed = "debian_12_lamp"

disk_size = "20G"
ram_size = "2G"
cpu_count = 1

backup_disk_size = "4G"
auto_rebuild = "weekly"

domains = ['xfennec.raydium.org']

prepare = [
    "admin@{core}/prepare/deb-comfort.sh",
    "admin@{core}/prepare/deb-lamp.sh",
    "app@{core}/prepare/wp-cli.sh",
]

# (done after prepare, and not replayed on VM rebuild)
install = [
    "app@{core}/install/deb-wordpress.sh",
]

backup = [
    "app@{core}/backup/lamp.sh",

    "app@{core}/backup/bash_history.sh",
    "admin@{core}/backup/bash_history.sh",
    "root@{core}/backup/bash_history.sh",
]

restore = [
    "app@{core}/restore/lamp.sh",
    "app@{core}/restore/../actions/wp_url.sh",

    "app@{core}/restore/bash_history.sh",
    "admin@{core}/restore/bash_history.sh",
    "root@{core}/restore/bash_history.sh",
]
```

See also the [complete sample VM configuration file](https://raw.github.com/OnitiFR/mulch/master/vm-samples/sample-vm-full.toml) to get a broader view of Mulch features.

Let's see a few interesting samples:

#### Domains

```toml
domains = ['test1.example.com']
redirect_to_https = true
redirects = [
    ["www.test1.example.com", "test1.example.com"],
]
```
Here, incoming requests for DNS domain `test1.example.com` will be proxied to this
VM on its port 80 as HTTP. You may use another destination port if needed, using `test1.example.com->8080` syntax.

Mulch will automatically generate a HTTPS certificate and redirect HTTP requests
to HTTPS (this is the default, `false` will proxy HTTP too). Requests to `www.test1.example.com` will be redirected
to `test1.example.com` (including HTTPS requests). All you have to do is target Mulch server in your DNS zone.

#### Scripts

Shell scripts are used at various stages of VM's lifecycle, let's have a
look at the *prepare* step.

```toml
# All scripts share the same base URL, so we use prepare_prefix_url:
prepare_prefix_url = "https://raw.githubusercontent.com/OnitiFR/mulch/master/scripts/prepare/"
prepare = [
    # user@script
    "admin@deb-comfort.sh",
    "admin@deb-lamp.sh",
]
```

During its creation, the VM will be prepared (see application lifecycle below) using
simple shell scripts. Each script is run as a specific user, either `admin` (with sudo
privileges) or `app` (who host the application). Script are downloaded from an URL. Mulch
provides a few sample scripts, but you're supposed to create and host your own specific scripts.

Here, a few comfort settings will be applied to the VM: installing
tools we like (powerline, Midnight Commander, …), creating a few command aliases, adding a nice motd, …

The other script will install and configure Apache, PHP and MariaDB, providing a ready-to-use
LAMP system. Environment variables are created with DB connection settings, htdocs directory, etc.

See [sample file](https://raw.github.com/OnitiFR/mulch/master/vm-samples/sample-vm-full.toml)
for more examples: origins, GIT support, etc.

#### Environment variables and secrets

It's also possible to define your own environment variables, providing various
settings to your application.

```toml
# Define system-wide environment variables
env = [
    ["TEST1", "foo"],
    ["TEST2", "bar"],
]

# This syntax also works:
env_raw = '''
TEST3=foo
TEST4="regular .env format"
'''
```

For secrets (API keys, passwords, …), you can use the `secrets` section:

```toml
# will inject OUR_API_KEY, SMTP_PASSWORD and ACCESS_KEY environment variables in the VM
secrets = [
    "OUR_API_KEY",
    "customer1/mail/SMTP_PASSWORD",
    "customer1/aws/ACCESS_KEY",
]
```

To define secrets, use the client : `mulch secret set customer1/mail/SMTP_PASSWORD` (the value is read from standard input)

How does it works exactly?
---
This schema shows the basic Mulch infrastructure:

![mulch infrastructure](https://raw.github.com/OnitiFR/mulch/master/doc/images/img_infra.png)

Mulch currently have 3 components :
 - `mulch`: the client
 - `mulchd`: the server daemon
 - `mulch-proxy`: the HTTP proxy

**Mulchd** receive requests from Mulch clients (REST API, HTTP(S), port `8686`) for VM management.
Application serving is done through HTTP(S) requests to **mulch-proxy** (ports `80` and `443`) and
SSH proxied access is done by mulchd (port `8022`).

VM have this lifecycle :

![mulch VMs lifecycle](https://raw.github.com/OnitiFR/mulch/master/doc/images/img_lifecycle.png)

 - **prepare** scripts: prepare the system for the application (install and configure web server, DB server, …)
 - **install** scripts: install and configure the application (ex: cURL download)
 - **backup** scripts: copy all important data: code, DB, configs, … During backup, a virtual disk is attached to the VM.
 - **restore** scripts: restore the application from the attached disk, copying back code, data, … (see these as backup "symmetric" scripts)

A new VM is channeled through *prepare* and *install* steps. If you create a
VM from a previous backup, *install* is replaced by *restore*. All steps are optional, but missing steps
may limit features.

Workflow examples:
 - WordPress: *prepare* a lamp system, *install* WordPress using cURL, *backup* and *restore* database and files
 - PHP application: *prepare* a lamp system and use `git clone`, `composer`, `yarn`, … (still in the *prepare* step) and *backup* and *restore* the database

Note that you can modify *cloud-init* step, but it's used internally by Mulch for VM provisioning (injecting
SSH keys, configure "home-phoning", …) and should have no interest to Mulch users.

VM revisions
---
A VM is not only identified by its name, but by its **name and a revision number**. This means you
can have several VMs sharing the same name, each with a different revision (`my-vm-r0`, `my-vm-r1`, …).

Only one revision can be **active** at a time. The active revision is the one the reverse proxy sends
requests to (it owns the VM's domains and exported ports), and it's the default target of every command
(`backup`, `lock`, `ssh`, …). This makes revisions the foundation of zero-downtime, "blue/green" style
deployments.

Building a new revision (`-n`) alongside the running one, without serving traffic yet (`-i`), is a
single command:
```sh
mulch vm create -n -i my-vm.toml
```
You can then test it (it has its own IP, SSH access, …) and switch over instantly when you're confident —
no rebuild, no downtime. If anything goes wrong, just activate the previous revision again to roll back.
```sh
mulch vm activate my-vm 1   # the reverse proxy switches over immediately
```

Show me more features!
---

#### HTTPS / Let's Encrypt
Here's the result of the previously linked `sample-vm-full.toml` configuration, showing automatic HTTPS certificates:

![mulch-proxy HTTPS Let's Encrypt certificates](https://raw.github.com/OnitiFR/mulch/master/doc/images/https_le.png)

You can of course provide your own certificates if needed.

#### SSH
Mulch allow easy SSH connection from mulch client with `mulch ssh` command. No configuration
is required and the client will retrieve your very own SSH key pair. You may select another user using
the `-u / --user` flag.

![mulch ssh](https://raw.github.com/OnitiFR/mulch/master/doc/images/mulch-ssh.png)

Another feature is SSH alisases generation. Simply call `mulch ssh-config` command, and aliases
for every VM will be generated. You can then use any usual OpenSSH command/feature: `ssh`, `scp`,
port forwarding, …

![mulch ssh-config](https://raw.github.com/OnitiFR/mulch/master/doc/images/mulch-ssh-config.png)

You can also **forward one of your own SSH keys** to a VM, so it becomes available in the VM's SSH agent
when you connect (handy to `git clone / push` a private repository, hop to another host, …):
```sh
mulch trust forward my-vm ~/.ssh/id_ed25519.pub
```
Mulch only stores the key's SHA256 fingerprint, never the key itself: the key is forwarded from your own
SSH agent, from any computer, as long as you use the same Mulch API key. Use `mulch trust list` to review
forwarded keys and `mulch trust remove` to revoke one. Note this is security-sensitive: while you're
connected, your key is reachable by anyone else on the VM.

#### Seeds
As said previously, seeds are base Linux images for VMs, defined in `mulchd.conf` server configuration
file:
```toml
[[seed]]
name = "ubuntu_2404"
url = "https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-amd64.img"
```

Mulchd will download images on first boot and each time the image is updated by the vendor.

Mulch requires OpenStack compliant images, and Cloud-Init 0.X is no more supported (so Debian 9 and less are out).

![mulch seed](https://raw.github.com/OnitiFR/mulch/master/doc/images/mulch-seed.png)

#### Seeders
You can create and maintain your own seeds. Mulch will create a ("seeder") VM using a TOML
file (based on another seed), prepare the VM, stop it and will then store its disk as
a seed (the VM is then deleted).

One usage of this feature is VM creation speedup, since you can pre-install packages.
See the following example ([ubuntu_2404_lamp.toml](https://raw.githubusercontent.com/OnitiFR/mulch/master/vm-samples/seeders/ubuntu_2404_lamp.toml)) :

```toml
[[seed]]
name = "ubuntu_2404_lamp"
seeder = "https://raw.githubusercontent.com/OnitiFR/mulch/master/vm-samples/seeders/ubuntu_2404_lamp.toml"
```
Seeds are automatically rebuild, so everything is kept up to date (base seed, packages, …).

#### Backups
Mulch provides a flexible backup / restore system for your applications and data:
archive, duplicate, iterate, CI/CD, …

You can even rebuild your entire VM from an updated seed in one command (see below).

![mulch vm backup](https://raw.github.com/OnitiFR/mulch/master/doc/images/mulch-vm-backup.png)

Backups are created by shell scripts (see VM lifecycle above). Backup scripts writes directly to an
attached (and mounted) disk. No need to create the backup **and then** copy it somewhere, all
is done is one step. Simpler, faster.

The backup format is **Qcow2**, a common virtual disk format. This format allows transparent
compression, so backup size is very close to the equivalent .tar.gz file.

Restoring a VM only requires a qcow2 backup file and the VM description file.

Since backup are virtual disks, they are writable. It's then easy to download, mount, **modify**
and upload back a backup to Mulch server in a few commands.

![mulch backup mount](https://raw.github.com/OnitiFR/mulch/master/doc/images/mulch-backup-mount.png)

#### VM rebuild
Using the backup system, Mulch provides a clean way to rebuild a VM entirely, using a transient
backup of itself.

![mulch vm rebuild](https://raw.github.com/OnitiFR/mulch/master/doc/images/mulch-vm-rebuild.png)

Again, the general idea behind Mulch is to secure Ops by industrializing and simplify such
processes ("service reconstructability").

You can configure auto-rebuild for each VM with `auto_rebuild` setting (daily, weekly,
monthly). We highly recommend this.

#### Reverse Proxy chaining
When using multiple Mulch instances, a frontal (standard) mulch-proxy can be configured to
forward traffic to children instances. It makes DNS configuration and VM migration between
mulch servers way easier. Thanks to an internal inter-proxy API, everything is managed
automatically: create your VM as usual and the parent mulch-proxy will instantly forward
requests. And domain name conflicts between instances are automatically checked too.

![mulch-proxy chaining](https://raw.github.com/OnitiFR/mulch/master/doc/images/img_chaining.png)

Here's what the parent mulch-proxy configuration will look like:
```toml
proxy_chain_mode = "parent"
proxy_chain_parent_url = "https://api.mydomain.tld:8787"
proxy_chain_psk = "MySecretPreShareKey123"
```

And here's the corresponding configuration for children:
```toml
proxy_chain_mode = "child"
proxy_chain_parent_url = "https://api.mydomain.tld:8787"
proxy_chain_child_url = "https://forward.server1.tld"
proxy_chain_psk = "MySecretPreShareKey123"
```

And that's it.

#### VM migration and secret sharing
In `mulchd.conf` config file, you can declare some other mulchd servers ("peers"), like so:
```toml
[[peer]]
name = "server2"
url = "https://server2.mydomain.tld:8686"
key = "K8OpSluPnUzcL2XipfPwt14WBT79aegqe4lZikObMIsiErqgxxco0iptr5MliQCY"
```
You can then move a VM from a server to another with minimal downtime :
```
mulch vm migrate my_vm server2
```

Peering also allow secret sharing between mulchd servers. Enable this feature
with `sync_secrets = true` in the peer section.

Secret sharing **must** be done in a bidirectional way:
 - each server must declare the all the others as peers, with `sync_secrets`
 - all servers must share the same encryption key (`mulch-secrets.key`)

#### Inter-VM communication
By default, network traffic is not allowed between VMs, but you can choose to
export a port from a VM to a group (group names starts with `@`).

Any other VM can then import the port from the group, and connect to this port (using an internal TCP-proxy).

To export a PostgreSQL server, use this in the VM TOML file:
```toml
ports = [
    "5432/tcp->@my_project",
]
```

Then, to import this port from another VM:
```toml
ports = [
    "5432/tcp<-@my_project",
]
```
Then, inside the VM, connect to `$_MULCH_PROXY_IP:$_5432_TCP`

Ports are dynamic, they're affected immediately by commands like `vm redefine` and `vm active`, no rebuild is needed. See sample TOML file for more informations.

#### Port forwarding
The TCP-proxy can also be used to expose a VM network port to the outside world using the special `PUBLIC` group:

```toml
# expose IRC server of this VM to the outside world
# (will listen on all host's interfaces)
ports = [
    "6667/tcp->@PUBLIC",
]
```

You can specify a different external port if needed:

```toml
# export our port 22 as public port 2222
ports = [
    "22/tcp->@PUBLIC:2222",
]
```

The TCP-proxy also supports [PROXY protocol](http://www.haproxy.org/download/1.8/doc/proxy-protocol.txt),
so you can preserve original IP address of the client if needed:

```toml
# export our port 22 as public port 2222
ports = [
    "22/tcp->@PUBLIC:2222 (PROXY)",
]
```

See `deb-proxy-proto.sh` "prepare" script to add transparent original IP forwarding
in your VMs even if your services don't support PROXY protocol natively.

#### Searching VMs
Once you host many VMs, `mulch vm search` filters them with a small query language instead of grepping
`vm list` output:
```sh
mulch vm search '(active == false && locked == false) || revision < 5'
mulch vm search 'like("*_prod")'
mulch vm search 'has_tag("wp-cli")'
```
Queries combine VM properties (state, seed, ram, author, …), boolean logic and a few helpers like
`has_domain()` or `has_script()`. Tag your VMs with the `tags` setting to make them even easier to group
and find.

#### Fine-grained API key rights
By default, an API key has full privileges. For shared or restricted access (CI/CD, a teammate, a
limited tool, …), you can attach explicit rights to a key, and it will then be limited to exactly what
you allow:
```sh
# this key may only list VMs and back them up
mulch key right add ci-key 'GET /vm'
mulch key right add ci-key 'POST /vm/* action=backup'
```
Rights map directly to API calls (with `*` wildcards allowed), and as soon as a key has one, everything
else is denied. The server log even shows denied requests, so building a tight right set is easy.

#### Rate limiting
The reverse proxy can rate-limit incoming HTTP(S) traffic to protect your VMs from spikes and abuse,
limiting both concurrent requests and request rate (with a configurable burst and a VIP exemption list).
You define named profiles in `mulchd.conf` and pick one per VM with the `rate_profile` setting — see the
sample config for the available knobs.

#### More…
You can lock a VM, so no "big" operation, like delete or rebuild can be done until the VM
is unlocked. Useful for production VMs.

![mulch vm locked](https://raw.github.com/OnitiFR/mulch/master/doc/images/mulch-vm-locked.png)

You still have the ability to use any libvirt tool, like virt-manager, to interact with VMs.

![virt-manager](https://raw.github.com/OnitiFR/mulch/master/doc/images/virt-manager.png)

You can also use 'do actions' for usual tasks. For instance `mulch do myvm db` will open your browser and automatically log you in phpMyAdmin (or any other db manager). And with included bash completion, such a command is just a matter of a few pressed keys!

How do I install the client?
---

Install go (sometimes named "golang") and then:
```sh
go install github.com/OnitiFR/mulch/cmd/mulch@latest
```
Usual Go requirements : check you have go/golang installed and `~/go/bin/` is in your `PATH` (or use `sudo ln -s /home/$USER/go/bin/mulch /usr/local/bin` if you have no idea about how to do this).

That's it, you can now run `mulch` command. It will show you a sample configuration file (`~/.mulch.toml`):
```toml
[[server]]
name = "my-mulch"
url = "http://192.168.10.104:8686"
key = "gein2xah7keeL33thpe9ahvaegF15TUL3surae3Chue4riokooJ5WuTI80FTWfz2"
```
Of course, you'll need to get your own API key / server URL (and set file mode to `0600`, key is private)

You can define multiple servers and use -s option to select one, or use
default = "my-mulch" as a global setting (i.e. before [[server]]).
First server is the default. Environment variable `SERVER` is also available.

To install bash completion, you may have a look at `mulch help completion`, you may
also install ssh aliases right now with `mulch ssh-config`.

![mulch client general help](https://raw.github.com/OnitiFR/mulch/master/doc/images/mulch-h.png)

How do I install the server? (mulchd and mulch-proxy)
---
*This section is still a WIP.*

### Quick install
For a quick installation on a **blank** Ubuntu 24.04 system, you may also use this standalone
auto-install script (with root privileges):
```
wget https://raw.github.com/OnitiFR/mulch/master/install/deb_ubuntu_autoinstall.sh
chmod +x deb_ubuntu_autoinstall.sh
./deb_ubuntu_autoinstall.sh
```

### Requirements

#### Ubuntu
```
sudo apt install golang-go
sudo apt install ebtables gawk libxml2-utils libcap2-bin dnsmasq libvirt-daemon-system libvirt-dev
sudo apt install git pkg-config build-essential qemu-kvm
sudo usermod -aG libvirt USER # replace USER by the user running mulchd
sudo setfacl -m g:libvirt-qemu:x /home/USER
```

#### Fedora
```
sudo dnf install golang git
sudo dnf install qemu-kvm libvirt-client libvirt-devel libvirt-daemon-kvm libvirt-daemon-config-nwfilter
sudo systemctl enable --now libvirtd
sudo usermod -aG libvirt USER # replace USER by the user running mulchd
sudo setfacl -m g:qemu:x /home/USER/
```

### Install
As a user:
 - `git clone https://github.com/OnitiFR/mulch.git`
 - `cd mulch`
 - `go install ./cmd/...`
 - `./install.sh --help` (see sample install section)

The install script will give you details about installation: destination, rights, services, …
