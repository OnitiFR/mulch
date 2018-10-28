# Mulch
Easy VM creation and management tool: velvet applications in an iron hypervisor

What is it?
---
Mulch is a light and practical virtual machine manager, it allows to create and host applications in VMs with a single command line and a simple description file. **You can see it as a simple KVM hardened container system**.

It's using libvirt API (KVM) and can share an existing libvirt server or use a dedicated one. It features a client-server architecture using a REST API. It's written in Go and use shell scripts to drive VM configuration and application installation. Base Linux images ("seeds") use Cloud-Init so almost every [OpenStack compliant image](https://docs.openstack.org/image-guide/obtain-images.html) will work out of the box. Default Mulch configuration provides seeds for Debian, Ubuntu and CentOS. (seed images will be downloaded and updated automatically)

Mulch features an embedded high-performance HTTP / HTTP2 reverse proxy with automatic Let's Encrypt certificate generation for VM hosted Web Application.

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

How about a real world example?
---
See the [complete sample VM configuration file](https://raw.github.com/Xfennec/mulch/master/vm-samples/sample-vm-full.toml) to get a broader view of Mulch features. We use this as a template for our "Wordpress farm" Mulch server VMs.

How does it works?
---
Here, a schema with mulch client, mulchd, mulch-proxy, libvirtd/KVM and VMs

Also, a schema with VM lifecycle (cloud-init, prepare, install, backup, restore)

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
Of course, you'll need to get your own API key / server URL.

How do I install the server? (mulchd and mulch-proxy)
---
Install:
 - libvirt daemon packages: libvirt-bin / libvirt(?)
 - development packages: libvirt-dev / libvirt-devel package needed
 - go get -u github.com/Xfennec/mulch/cmd/...
 - cd go/src/github.com/Xfennec/mulch
 - ./install.sh
