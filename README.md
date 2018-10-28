# Mulch
Easy VM creation and management tool: velvet applications in an iron hypervisor

What is it?
---
Mulch is a light and practical virtual machine manager, it allows to create and host applications in VMs with
a single command line and a simple description file. **You can see it as a simple KVM hardened container system**.

What's the point?
---
It aims people and organizations who don't want to (or can't) invest too much time in creating their own
private cloud, but want a simple and performant solution.

It features a client-server architecture using a REST API. It's written in Go and use shell scripts to drive VM
configuration and application installation.

Mulch is using libvirt API (KVM) and can share an existing libvirt server or use a dedicated one. No libvirt
configuration (or knowledge) is required, Mulch will create and manage needed resources (storage, network).
You will only interact with Mulch.

![Tech banner](https://raw.github.com/Xfennec/mulch/master/doc/images/tech-banner.png)

Base Linux images ("seeds") use Cloud-Init so almost every [OpenStack compliant image](https://docs.openstack.org/image-guide/obtain-images.html) will work out of the box. Default Mulch
configuration provides seeds for Debian, Ubuntu and CentOS. Seed images will be downloaded and updated
automatically, it is Mulch's job, not yours.

Mulch features an embedded high-performance HTTP / HTTP2 reverse proxy with automatic Let's Encrypt certificate
generation for VM hosted Web Application.

Mulh also provides an SSH proxy providing full access (ssh, scp, port forwarding, …) to VMs for Mulch users.

Show me!
---
Here is a minimal VM description file: (TOML format)
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

![mulch vm create minimal](https://raw.github.com/Xfennec/mulch/master/doc/images/mulch-create-mini.png)

Here, the VM is up and ready 40 seconds later.

How about a real world example?
---
See the [complete sample VM configuration file](https://raw.github.com/Xfennec/mulch/master/vm-samples/sample-vm-full.toml) to get a broader view of Mulch features. We use this as a template for our "Wordpress farm" Mulch server VMs.

Here is a few interesting samples:

```toml
domains = ['test1.example.com']
redirect_to_https = true
redirects = [
    ["www.test1.example.com", "test1.example.com"],
]
```
Here, incoming requests for DNS domain `test1.example.com` will be proxied to this
VM on its port 80 as HTTP. You may use another port if needed, using `test1.example.com->8080` syntax.

Mulch will automatically generate a HTTPS certificate, and HTTP requests will be redirected
to HTTPS (this is the default anyway). Requests to `www.test1.example.com` will be redirected
to `test1.example.com` (including HTTPS requests).

```toml
# If all prepare scripts share the same base URL, you can use prepare_prefix_url.
# Otherwise, use absolute URL in 'prepare': admin@https://server/script.sh
prepare_prefix_url = "https://raw.githubusercontent.com/Xfennec/mulch/master/scripts/prepare/"
prepare = [
    # user@script
    "admin@deb-comfort.sh",
    "admin@deb-lamp.sh",
]
```

During its creation, the VM will be prepared (see application lifecycle below) using
simple shell scripts. Each script is run with a specific user, either `admin` with sudo
privileges, or `app`, the user who host the application. Script are downloaded from an URL.

Here, a few comfort settings will be applied to the VM: installing
tools we like powerline and Midnight Commander, creating a few alisases, adding a nice motd, …

The other script will install and configure Apache, PHP and MariaDB, providing a ready-to-use
LAMP system. Environment variables are created with DB connection settings, htdoc directory, etc.

```toml
# Define system-wide environment variables
env = [
    ["TEST1", "foo"],
    ["TEST2", "bar"],
]
```
It's also possible to define your own environment variables, providing various
settings and secrets to your application.

How does it works exactly?
---
This schema show the basic Mulch infrastructure:

![mulch infrastructure](https://raw.github.com/Xfennec/mulch/master/doc/images/img_infra.png)


Also, a schema with VM lifecycle (cloud-init, prepare, install, backup, restore)

Show me some more features!
---
https
ssh (admin, app)
seed list
backup
rebuild
lock

How do I install the client?
---
Usual Go requirements : check you have go/golang installed and `~/go/bin/` is in your `PATH` (or copy binary in one of your `PATH` directories).

Then install the client:
```sh
go get -u github.com/Xfennec/mulch/cmd/mulch
```

That's it, you can now run `mulch` command. It will show you a sample configuration file (`~/.mulch.toml`):
```toml
[[server]]
name = "my-mulch"
url = "http://192.168.10.104:8585"
key = "gein2xah7keeL33thpe9ahvaegF15TUL3surae3Chue4riokooJ5WuTI80FTWfz2"
```
Of course, you'll need to get your own API key / server URL (and set file mode to `0600`, key is private)

![mulch client general help](https://raw.github.com/Xfennec/mulch/master/doc/images/mulch-h.png)

How do I install the server? (mulchd and mulch-proxy)
---
Install:
 - libvirt daemon packages: libvirt-bin / libvirt(?)
 - development packages: libvirt-dev / libvirt-devel package needed
 - go get -u github.com/Xfennec/mulch/cmd/...
 - cd go/src/github.com/Xfennec/mulch
 - ./install.sh
