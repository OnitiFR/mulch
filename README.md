# mulch
Easy VM creation and management tool: velvet applications in an iron hypervisor

Mulch is a light and practical virtual machine manager, using
libvirt API. It can share an existing libvirt server or use a
dedicated one. It features a client-server architecture using
a REST API.

Install:
 - libvirt-bin / libvirt(?)
 - libvirt-dev / libvirt-devel package needed
 - go get -u github.com/Xfennec/mulch/cmd/...
 - cd go/src/github.com/Xfennec/mulch
 - ./install.sh
