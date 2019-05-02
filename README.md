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
 - [Show me more features!](#show-me-more-features)
 - [How do I install the client?](#how-do-i-install-the-client)
 - [How do I install the server?](#how-do-i-install-the-server-mulchd-and-mulch-proxy)

What's the point?
---
It aims people and organizations who don't want to (or can't) invest too much time in creating their own
private cloud, but want a simple solution with great performance.

It features a client-server architecture using a REST API. It's written in Go and use shell scripts to drive VM
configuration and application installation.

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
seed = "debian_9"

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
See the [complete sample VM configuration file](https://raw.github.com/OnitiFR/mulch/master/vm-samples/sample-vm-full.toml) to get a broader view of Mulch features. We use this as a template for our "Wordpress farm" Mulch server VMs.

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
# If all prepare scripts share the same base URL, you can use prepare_prefix_url.
# Otherwise, use absolute URL in 'prepare': admin@https://server/script.sh
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

#### Environment variables

It's also possible to define your own environment variables, providing various
settings and secrets to your application.

```toml
# Define system-wide environment variables
env = [
    ["TEST1", "foo"],
    ["TEST2", "bar"],
]
```

How does it works exactly?
---
This schema shows the basic Mulch infrastructure:

![mulch infrastructure](https://raw.github.com/OnitiFR/mulch/master/doc/images/img_infra.png)

Mulch currently have 3 components :
 - `mulch`: the client
 - `mulchd`: the server daemon
 - `mulch-proxy`: the HTTP proxy

**Mulchd** receive requests from Mulch clients (REST API, HTTP, port `8585`) for VM management.
Application serving is done through HTTP(S) requests to **mulch-proxy** (ports `80` and `443`) and
SSH proxied access is done by mulchd (port `8022`).

VM have this lifecycle :

![mulch VMs lifecycle](https://raw.github.com/OnitiFR/mulch/master/doc/images/img_lifecycle.png)

 - **prepare** scripts: prepare the system for the application (install and configure web server, DB server, …)
 - **install** scripts: install and configure the application (download / git clone, composer, yarn, …)
 - **backup** scripts: copy all important data: code, DB, configs, … During backup, a virtual disk is attached to the VM.
 - **restore** scripts: restore the application from the attached disk, copying back code, data, … (see these as backup "symmetric" scripts)

A new VM is channeled through *prepare* and *install* steps. If you create a
VM from a previous backup, *install* is replaced by *restore*. All steps are optional, but missing steps
will limit features.

Note that you can modify *cloud-init* step, but it's used internally by Mulch for VM provisioning (injecting
SSH keys, configure "home-phoning", …) and should have no interest to Mulch users.

Show me more features!
---

#### HTTPS / Let's Encrypt
Here's the result of the previously linked `sample-vm-full.toml` configuration, showing automatic HTTPS certificates:

![mulch VMs lifecycle](https://raw.github.com/OnitiFR/mulch/master/doc/images/https_le.png)

#### SSH
Mulch allow easy SSH connection from mulch client with `mulch ssh` command. No configuration
is required and the client will retrieve your very own SSH key pair. You may select another user using
the `-u / --user` flag.

![mulch ssh](https://raw.github.com/OnitiFR/mulch/master/doc/images/mulch-ssh.png)

Another feature is SSH alisases generation. Simply call `mulch ssh-config` command, and aliases
for every VM will be generated. You can then use any usual OpenSSH command/feature: `ssh`, `scp`,
port forwarding, …

![mulch ssh-config](https://raw.github.com/OnitiFR/mulch/master/doc/images/mulch-ssh-config.png)

#### Seeds
As said previously, seeds are base Linux images for VMs, defined in `mulchd.conf` server configuration
file:
```toml
[[seed]]
name = "ubuntu_1810"
current_url = "http://cloud-images.ubuntu.com/cosmic/current/cosmic-server-cloudimg-amd64.img"
as = "ubuntu-1810-amd64.qcow2"
```

 Mulchd will download images on first boot and each time the image is updated by the vendor.

 ![mulch seed](https://raw.github.com/OnitiFR/mulch/master/doc/images/mulch-seed.png)

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
Using the backup system, Mulch provides a clean way to rebuild a VM entirely. Rebuilding implies
the following steps: **backup** scripts, **clone** of the original VM (will be used if rebuild fails),
**delete** original VM, create a **new VM** with the same description and finally, **restore** the VM using
the backup. The backup is transient and will be deleted.

![mulch vm rebuild](https://raw.github.com/OnitiFR/mulch/master/doc/images/mulch-vm-rebuild.png)

Again, the general idea behind Mulch is to secure Ops by industrializing and simplify such
processes ("service reconstructability").

#### More…
You can lock a VM, so no "big" operation, like delete or rebuild can be done until the VM
is unlocked. Useful for precious VMs.

![mulch vm locked](https://raw.github.com/OnitiFR/mulch/master/doc/images/mulch-vm-locked.png)

You still have the ability to use any libvirt tool, like virt-manager, to interact with VMs.

![virt-manager](https://raw.github.com/OnitiFR/mulch/master/doc/images/virt-manager.png)

You can also use 'do actions' for usual tasks. For instance `mulch do myvm db` will open your browser and automatically log you in phpMyAdmin (or any other db manager). And with included bash completion, such a command is just a matter of a few key presses !

How do I install the client?
---
Usual Go requirements : check you have go/golang installed and `~/go/bin/` is in your `PATH` (or copy/link binary in one of your `PATH` directories; use `sudo ln -s /home/$USER/go/bin/mulch /usr/local/bin` if you have no idea of how to do this).

Then install the client:
```sh
go get -u github.com/OnitiFR/mulch/cmd/mulch
```

That's it, you can now run `mulch` command. It will show you a sample configuration file (`~/.mulch.toml`):
```toml
[[server]]
name = "my-mulch"
url = "http://192.168.10.104:8585"
key = "gein2xah7keeL33thpe9ahvaegF15TUL3surae3Chue4riokooJ5WuTI80FTWfz2"
```
Of course, you'll need to get your own API key / server URL (and set file mode to `0600`, key is private)

You can define multiple servers and use -s option to select one, or use
default = "my-mulch" as a global setting (i.e. before [[server]]).
First server is the default. Environment variable `SERVER` is also available.

![mulch client general help](https://raw.github.com/OnitiFR/mulch/master/doc/images/mulch-h.png)

To install bash completion, you may have a look at `mulch help completion`.

How do I install the server? (mulchd and mulch-proxy)
---
*This section is still a WIP.*

### Requirements:

#### Ubuntu: (19.04 / 18.10 / 18.04)
```
sudo apt install golang-go
sudo apt install ebtables gawk libxml2-utils libcap2-bin dnsmasq libvirt-daemon-system libvirt-dev
```

#### Fedora:
```
sudo dnf install golang git
sudo dnf install qemu-kvm libvirt-client libvirt-devel libvirt-daemon-kvm libvirt-daemon-config-nwfilter
sudo systemctl enable --now libvirtd
sudo usermod -aG libvirt USER # replace USER by the user running mulchd
sudo setfacl -m g:qemu:x /home/USER/
```

#### Debian 9: (Unsupported! libvirt is too old [missing disk aliases])
Download and install a newer release of Go than provided
packages, see https://golang.org/dl/
```
sudo apt install build-essential pkg-config git gawk libxml2-utils
sudo apt install qemu-kvm ebtables libcap2-bin bridge-utils dnsmasq libvirt-daemon-system libvirt-dev
sudo usermod -aG libvirt USER # replace USER by the user running mulchd
```


### Install:
As a user:
 - `go get -u github.com/OnitiFR/mulch/cmd/...`
 - `cd go/src/github.com/OnitiFR/mulch`
 - `./install.sh --help` (see sample install section)

The install script will give you details about installation: destination, rights, services, …


### Quick demo install:
For a quick demo installation on a **blank** Ubuntu system, you may also use this standalone
auto-install script (with root privileges):
```
wget https://raw.github.com/OnitiFR/mulch/master/install/ubuntu_autoinstall.sh
chmod +x ubuntu_autoinstall.sh
./ubuntu_autoinstall.sh
```
