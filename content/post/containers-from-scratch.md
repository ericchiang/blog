+++
title = "Containers from Scratch"
date = "2016-01-07"
+++

This is write up for talk I gave at [CAT BarCamp][cat-barcamp] an awesome unconference at Portland State University. The talk started with the self-imposed challenge _"an intro to containers without rkt or Docker"_.

Often thought of as cheap VMs, containers are just isolated programs running on a single host. The actually isolation comes from several underlying technologies built into the Linux kernel used to restrict groups of processes. Namespaces, cgroups, chroots and lots of terms you've probably heard before.

So, let's have a little fun and use those underlying technologies to build our own containers.

On today's agenda:

* [setting up a file system](#container-file-system)
* [chroot](#chroot)
* [unshare](#creating-namespaces-with-unshare)
* [nsenter](#entering-namespaces-with-nsenter)
* [bind mounts](#getting-around-chroot-with-bind-mounts)
* [cgroups](#cgroups)
* [capabilities](#container-security-capabilities)
* [conclusion](#conclusion)

## Container file systems

Container images, the thing you download from the internet, are literally just tarballs (or tarballs in tarballs if you're fancy). The least magic part of a container are the files you interact with.

For this post I've build a simple tarball using a Docker. The tarball holds a something that looks like a Debian filesystem and will be our playground for isolating processes.

```
$ wget https://github.com/ericchiang/containers-from-scratch/releases/download/v0.1.0/rootfs.tar.gz
$ sha256sum rootfs.tar.gz 
c79bfb46b9cf842055761a49161831aee8f4e667ad9e84ab57ab324a49bc828c  rootfs.tar.gz
```

First, explode the tarball and poke around.

```
$ # tar needs sudo to create /dev files and setup file ownership
$ sudo tar -zxf rootfs.tar.gz
$ ls rootfs
bin   dev  home  lib64  mnt  proc  run   srv  tmp  var
boot  etc  lib   media  opt  root  sbin  sys  usr
$ ls -al rootfs/bin/ls
-rwxr-xr-x. 1 root root 118280 Mar 14  2015 rootfs/bin/ls
```

The resulting directory looks an awful lot like a Linux system. There's a `bin` directory with executables, an `etc` with system configuration, a `lib` with shared libraries, etc.

Since I've glossed over how to actually build this tarball, I'll strongly recommend the [_"Minimal Containers"_][minimal-containers] talk by my coworker Brian Redbeard.

## chroot

The first tool we'll be working with is `chroot`. A thin wrapper around the similarly named syscall that allows us to restrict the view of the file system for a process. In this case, we'll restrict our process to the "rootfs" directory then exec a shell.

Once we're in there, we can poke around, run commands, and do typical shell things.

```
$ sudo chroot rootfs /bin/bash
root@localhost:/# ls /
bin   dev  home  lib64  mnt  proc  run   srv  tmp  var
boot  etc  lib   media  opt  root  sbin  sys  usr
root@localhost:/# which python
/usr/bin/python
root@localhost:/# /usr/bin/python -c 'print "Hello, container world!"'
Hello, container world!
root@localhost:/# 
```

It's worth noting that this works because of all the things baked into the tarball. When we execute the Python interpreter, we're executing `rootfs/usr/bin/python`, not the host's Python. That interpreter depends on shared libraries and device files that have been intentionally included in the archive.

Despite all of these weird requirements, containers excel because all of these dependencies can be bundled statically. Consumers download a tarball that holds all the hard work of figuring out what it takes to run an application beforehand. 

Speaking of applications, instead of shell we can run one in our chroot.

```
$ sudo chroot rootfs python -m SimpleHTTPServer
Serving HTTP on 0.0.0.0 port 8000 ...
```

If you're following along at home, you'll be able to view everything the server can see at [http://localhost:8000/](http://localhost:8000/).

## Creating namespaces with unshare

How isolated is this chroot'd process? Let's run a top command on the host in another terminal.

```
$ # outside of the chroot
$ top
```

Sure enough, in the chroot we can see the "top" invocation.

```
$ sudo chroot rootfs /bin/bash
root@localhost:/# mount -t proc proc /proc
root@localhost:/# ps aux | grep top
1000     24753  0.1  0.0 156636  4404 ?        S+   22:28   0:00 top
root     24764  0.0  0.0  11132   948 ?        S+   22:29   0:00 grep top
```

Bet yet, our chrooted shell is running as root, so it has no problem killing it.

```
root@localhost:/# pkill top
```

So much for containment.

This is where we get to talk about namespaces. Namespaces are the chroots of things like the process tree, network interfaces, and mounts allowing us to restrict a process' view of these systems.

Creating namespace is super easy, just a single syscall with one argument, [`unshare`](https://linux.die.net/man/2/unshare).

The `unshare` command line tool gives us a nice wrapper around this syscall and lets us setup namespaces manually. In this case, we'll create a PID namespace for the shell.

```
$ # -p to create a PID namespace, -f to fork, --mount-proc remounts /proc
$ sudo unshare -p -f --mount-proc=$PWD/rootfs/proc chroot rootfs /bin/bash
root@localhost:/# ps aux
USER       PID %CPU %MEM    VSZ   RSS TTY      STAT START   TIME COMMAND
root         1  0.0  0.0  20268  3240 ?        S    22:34   0:00 /bin/bash
root         2  0.0  0.0  17504  2096 ?        R+   22:34   0:00 ps aux
root@localhost:/#
```

Poking around we can see that our process thinks it's PID 1. Better yet, it can't see the host's process tree.

## Entering namespaces with nsenter

A powerful aspect of namespaces is their composability; processes may choose to separate some namespaces but share others. For instance it may be useful for two programs to have isolated PID namespaces, but [share a network namespace][k8s-pods]. This brings us to the [`setns`][setns] syscall and the `nsenter`command line tool.

Let's find the shell running in a chroot from our last example.

```
$ ps aux | grep /bin/bash | grep root
...
root     29840  0.0  0.0  20272  3064 pts/5    S+   17:25   0:00 /bin/bash
```

The kernel exposes namespaces under `/proc/(PID)/ns` as files. In this case, `/proc/29840/ns/pid` is the process namespace we're hoping to join.

```
$ sudo ls -l /proc/29840/ns
total 0
lrwxrwxrwx. 1 root root 0 Oct 15 17:31 ipc -> 'ipc:[4026531839]'
lrwxrwxrwx. 1 root root 0 Oct 15 17:31 mnt -> 'mnt:[4026532434]'
lrwxrwxrwx. 1 root root 0 Oct 15 17:31 net -> 'net:[4026531969]'
lrwxrwxrwx. 1 root root 0 Oct 15 17:31 pid -> 'pid:[4026532446]'
lrwxrwxrwx. 1 root root 0 Oct 15 17:31 user -> 'user:[4026531837]'
lrwxrwxrwx. 1 root root 0 Oct 15 17:31 uts -> 'uts:[4026531838]'
```

The `nsenter` command provides a wrapper around `setns` to enter a namespace. We'll provide the namespace file, then run the `unshare` to remount `/proc` and `chroot` to setup a chroot.

```
$ sudo nsenter --pid=/proc/29840/ns/pid \
    unshare -f --mount-proc=$PWD/rootfs/proc \
    chroot rootfs /bin/bash
root@localhost:/# ps aux
USER       PID %CPU %MEM    VSZ   RSS TTY      STAT START   TIME COMMAND
root         1  0.0  0.0  20272  3064 ?        S+   00:25   0:00 /bin/bash
root         5  0.0  0.0  20276  3248 ?        S    00:29   0:00 /bin/bash
root         6  0.0  0.0  17504  1984 ?        R+   00:30   0:00 ps aux
```

Having entered the namespace successfully, when we run `ps` in the second shell we see the first.

## Getting around chroot with bind mounts

We make a new directory to mount into the chroot and write a file.

```
$ sudo mkdir readonlyfiles
$ echo "hello" > readonlyfiles/hi.txt
```

Next, we'll create a target directory in our container and bind mount the directory in.

```
$ sudo mkdir -p rootfs/var/readonlyfiles
$ sudo mount --bind -o $PWD/readonlyfiles $PWD/rootfs/var/readonlyfiles
```

The chroot'd process can now see the mounted files.

```
$ sudo chroot rootfs /bin/bash
root@localhost:/# cat /var/readonlyfiles/hi.txt
hello
```

However, we can't write them.

```
root@localhost:/# echo "bye" > /var/readonlyfiles/hi.txt
bash: /var/readonlyfiles/hi.txt: Read-only file system
```

Mounts provide the ability to inject files or persistent space to a container without editing the actual container image. We can provide simple things like onfiguration or credentials through files, or even persistent volumes like NFS.

Remove the mount using `umount` (`rm` won't work).

```
$ sudo umount $PWD/rootfs/var/readonlyfiles
```

## cgroups

cgroups, short for control groups, allow kernel imposed isolation on resources like memory and CPU. After all, what's the point of isolating processes they can still kill neighbors by hogging RAM?

The kernel exposes cgroups through a magical `/sys/fs/cgroup` directory. If your machine doesn't have one you may have to [mount the memory cgroup][mount-cgroups] to follow along.

```
$ ls /sys/fs/cgroup/
blkio  cpuacct      cpuset   freezer  memory   net_cls,net_prio  perf_event  systemd
cpu    cpu,cpuacct  devices  hugetlb  net_cls  net_prio          pids
```

For this example we'll create a cgroup to restrict the memory of a process. Creating a cgroup is easy, just create a directory and the kernel fills it with configuration files. In this case we'll create a memory cgroup called "demo".

```
$ sudo su
# mkdir /sys/fs/cgroup/memory/demo
# ls /sys/fs/cgroup/memory/demo/
cgroup.clone_children               memory.memsw.failcnt
cgroup.event_control                memory.memsw.limit_in_bytes
cgroup.procs                        memory.memsw.max_usage_in_bytes
memory.failcnt                      memory.memsw.usage_in_bytes
memory.force_empty                  memory.move_charge_at_immigrate
memory.kmem.failcnt                 memory.numa_stat
memory.kmem.limit_in_bytes          memory.oom_control
memory.kmem.max_usage_in_bytes      memory.pressure_level
memory.kmem.slabinfo                memory.soft_limit_in_bytes
memory.kmem.tcp.failcnt             memory.stat
memory.kmem.tcp.limit_in_bytes      memory.swappiness
memory.kmem.tcp.max_usage_in_bytes  memory.usage_in_bytes
memory.kmem.tcp.usage_in_bytes      memory.use_hierarchy
memory.kmem.usage_in_bytes          notify_on_release
memory.limit_in_bytes               tasks
memory.max_usage_in_bytes
```

We'll use the magic of the `echo` command to limit the cgroup to 100MB and turn off swap.

```
# echo "100000000" > /sys/fs/cgroup/memory/demo/memory.limit_in_bytes
# echo "0" > /sys/fs/cgroup/memory/demo/memory.swappiness
```

Processes are assigned to cgroups through the `tasks` file which contains a list of PIDs. We can join the cgroup by writing our own PID.

```
# echo $$ > /sys/fs/cgroup/memory/demo/tasks
```

Finally we need a memory hungry application.

```python
f = open("/dev/urandom", "r")
data = ""

i=0
while True:
    data += f.read(10000000) # 10mb
    i += 1
    print "%dmb" % (i*10,)
```

If you've setup the cgroup correctly, this program won't crash your computer.

```
# python hungry.py
10mb
20mb
30mb
40mb
50mb
60mb
70mb
80mb
Killed
```

If you're still reading, congratulations!

cgroups can't be removed until every processes in the `tasks` file has exited or been reassigned. Exit the shell and remove the directory with `rmdir` (don't use `rm -r`).

```
# exit
exit
$ sudo rmdir /sys/fs/cgroup/memory/demo
```

## Container security: capabilities

Containers are extremely effective ways of running arbitrary code from the internet as root, and this is where the low overhead of containers hurts us. Containers are significantly easier to break out of than a VM. As a result many technologies used to improve the security of containers, such as SELinux, seccomp, and capabilities involve limiting the power of processes already running as root.

In this section we'll be exploring Linux capabilities.

Consider the following Go program which attempts to listen on port 80.

```go
package main

import (
    "fmt"
    "net"
    "os"
)

func main() {
    if _, err := net.Listen("tcp", ":80"); err != nil {
        fmt.Fprintln(os.Stdout, err)
        os.Exit(2)
    }
    fmt.Println("success")
}
```

What happens when we compile and run this?

```
$ go build -o listen listen.go
$ ./listen
listen tcp :80: bind: permission denied
```

Predictably this program fails; listing on port 80 requires permissions we don't have. Of course we can just use `sudo`, but we'd like to give the binary just the one permission to listen on lower ports.

Capabilities are a set of discrete powers that together make up everything root can do. This can be seemingly mundane things like setting the system clock, or the ability to kill arbitrary processes. In this case, `CAP_NET_BIND_SERVICE` allows executables to listen on lower ports.

We can grant the executable `CAP_NET_BIND_SERVICE` using the `setcap` command.

```
$ sudo setcap cap_net_bind_service=+ep listen
$ getcap listen
listen = cap_net_bind_service+ep
$ ./listen
success
```

For things already running as root, we're more interested in taking capabilities away than granting them. First let's see all powers our root shell has:

```
$ sudo su
# capsh --print
Current: = cap_chown,cap_dac_override,cap_dac_read_search,cap_fowner,cap_fsetid,cap_kill,cap_setgid,cap_setuid,cap_setpcap,cap_linux_immutable,cap_net_bind_service,cap_net_broadcast,cap_net_admin,cap_net_raw,cap_ipc_lock,cap_ipc_owner,cap_sys_module,cap_sys_rawio,cap_sys_chroot,cap_sys_ptrace,cap_sys_pacct,cap_sys_admin,cap_sys_boot,cap_sys_nice,cap_sys_resource,cap_sys_time,cap_sys_tty_config,cap_mknod,cap_lease,cap_audit_write,cap_audit_control,cap_setfcap,cap_mac_override,cap_mac_admin,cap_syslog,cap_wake_alarm,cap_block_suspend,37+ep
Bounding set =cap_chown,cap_dac_override,cap_dac_read_search,cap_fowner,cap_fsetid,cap_kill,cap_setgid,cap_setuid,cap_setpcap,cap_linux_immutable,cap_net_bind_service,cap_net_broadcast,cap_net_admin,cap_net_raw,cap_ipc_lock,cap_ipc_owner,cap_sys_module,cap_sys_rawio,cap_sys_chroot,cap_sys_ptrace,cap_sys_pacct,cap_sys_admin,cap_sys_boot,cap_sys_nice,cap_sys_resource,cap_sys_time,cap_sys_tty_config,cap_mknod,cap_lease,cap_audit_write,cap_audit_control,cap_setfcap,cap_mac_override,cap_mac_admin,cap_syslog,cap_wake_alarm,cap_block_suspend,37
Securebits: 00/0x0/1'b0
 secure-noroot: no (unlocked)
 secure-no-suid-fixup: no (unlocked)
 secure-keep-caps: no (unlocked)
uid=0(root)
gid=0(root)
groups=0(root)
```

Yeah, that's a lot of capabilities.

As an example, we'll drop a bunch of capabilities using `capsh` including `CAP_CHOWN`. If things work as expected, our shell shouldn't be able to modify file ownership.

```
$ sudo capsh --drop=cap_chown,cap_setpcap,cap_setfcap,cap_sys_admin --chroot=$PWD/rootfs --
root@localhost:/# whoami
root
root@localhost:/# chown nobody /bin/ls
chown: changing ownership of '/bin/ls': Operation not permitted
```

## Conclusion

Yes, there are a ton of things we didn't cover here. But before we get into that list, I think it's important to stress some points.

Containers aren't magic. Anyone with a Linux machine can play around with them and tools like Docker, rkt, or LXC are just wrappers around things built into every modern kernel. No, you probably shouldn't go and implement your own container runtime. But having a better understanding of these lower level technologies will help you work with (and especially debug) these higher level tools.

There's a ton we didn't cover here.

[cat-barcamp]: http://catbarcamp.org/
[minimal-containers]: https://www.youtube.com/watch?v=gMpldbcMHuI
[k8s-pods]: http://kubernetes.io/docs/user-guide/pods/
[setns]: https://linux.die.net/man/2/setns
[mount-cgroups]: https://access.redhat.com/documentation/en-US/Red_Hat_Enterprise_Linux/6/html/Resource_Management_Guide/sec-memory.html#memory_example-usage
