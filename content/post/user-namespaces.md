+++
title = "User Namespaces"
date = "2019-02-20"
+++

Ian Coldwater recently had a great thread on [bridging the gap between the security and container worlds][container-tweet]. A lot of those answers wont fit in a tweet, so here’s my attempt for a more in-depth response.

[container-tweet]: https://twitter.com/IanColdwater/status/1097036373465407489 
## Namespaces for everyone
User namespaces are a way to create unique views of user and group IDs. Unlike [other namespaces](../containers-from-scratch/), they can be created by non-root users and are primarily used by unprivileged processes to access capabilities normally reserved for root.

The `unshare` tool can be used to create a user namespace and act as root:

```
$ whoami
ericchiang
$ unshare --map-root-user
# whoami
root
```
Under the hood, `unshare` is calling [`unshare(2)`][unshare-2] and `fork(2)`, modifying the UID and GID mappings files under `/proc` , then exec’ing a shell:

```
$ unshare --map-root-user
# cat /proc/self/uid_map
         0       1000          1
# cat /proc/self/gid_map
         0       1000          1
```

The map files are formatted as follows. In the case above, `0` (root) for the target ID, `1000` (my PID/GID) for the source ID, and `1` to only map a single UID and GID:

```
[target ID] [source ID] [ID range]
```

Within a user namespace, a process can run commands as if it was root. For example, an unprivileged user can call `chroot`:

```
$ # create a root file system
$ mkdir rootfs
$ sudo debootstrap stable rootfs http://deb.debian.org/debian/
…
$ unshare --map-root-user chroot rootfs # run without sudo
# ls /
bin  boot  dev  etc  home  lib  lib64  media  mnt
opt  proc  root  run  sbin  srv  sys  tmp  usr  var
```

Or setup other namespaces and mount filesystems:

```
$ # create a user, mount, and PID namespace
$ unshare --map-root-user --mount --pid --fork
# mount -t proc proc $PWD/rootfs/proc
# chroot rootfs
# ps
    PID TTY          TIME CMD
      1 ?        00:00:00 bash
      3 ?        00:00:00 ps
```

But are we root? We certainly have a lot of capabilities:

```
$ unshare --map-root-user capsh --print
Current: = cap_chown,cap_dac_override,cap_dac_read_search,cap_fowner,cap_fsetid,cap_kill,cap_setgid,cap_setuid,cap_setpcap,cap_linux_immutable,cap_net_bind_service,cap_net_broadcast,cap_net_admin,cap_net_raw,cap_ipc_lock,cap_ipc_owner,cap_sys_module,cap_sys_rawio,cap_sys_chroot,cap_sys_ptrace,cap_sys_pacct,cap_sys_admin,cap_sys_boot,cap_sys_nice,cap_sys_resource,cap_sys_time,cap_sys_tty_config,cap_mknod,cap_lease,cap_audit_write,cap_audit_control,cap_setfcap,cap_mac_override,cap_mac_admin,cap_syslog,cap_wake_alarm,cap_block_suspend,cap_audit_read+ep
Bounding set =cap_chown,cap_dac_override,cap_dac_read_search,cap_fowner,cap_fsetid,cap_kill,cap_setgid,cap_setuid,cap_setpcap,cap_linux_immutable,cap_net_bind_service,cap_net_broadcast,cap_net_admin,cap_net_raw,cap_ipc_lock,cap_ipc_owner,cap_sys_module,cap_sys_rawio,cap_sys_chroot,cap_sys_ptrace,cap_sys_pacct,cap_sys_admin,cap_sys_boot,cap_sys_nice,cap_sys_resource,cap_sys_time,cap_sys_tty_config,cap_mknod,cap_lease,cap_audit_write,cap_audit_control,cap_setfcap,cap_mac_override,cap_mac_admin,cap_syslog,cap_wake_alarm,cap_block_suspend,cap_audit_read
Securebits: 00/0x0/1'b0
 secure-noroot: no (unlocked)
 secure-no-suid-fixup: no (unlocked)
 secure-keep-caps: no (unlocked)
uid=0(root)
gid=0(root)
groups=65534(nogroup),65534(nogroup),65534(nogroup),0(root)
```

The truth is, once the process attempts to interact with a resource outside of the namespace, “real root” restrictions apply. For example, the process can’t listen on a privileged port:

```
$ unshare --map-root-user nc -l -p 80
Can't grab 0.0.0.0:80 with bind : Permission denied
```

Or override host file permissions:

```
$ unshare --map-root-user cat /etc/shadow
cat: /etc/shadow: Permission denied
```

Ultimately, the process is only root in the namespace and host restrictions still apply as if it was running as a normal user.

[unshare-2]: https://linux.die.net/man/2/unshare
## Use in practice
In practice, user namespaces are largely used for unprivileged management of namespaces.

[Flatpak][flatpak], a tool for running containerized desktop applications, allows unprivileged users to run containers by leveraging user namespaces to create the application’s sandbox. Flatpak’s runtime [bubblewrap][bwrap] is also used by projects like GNOME Desktop to isolate risky processes. 

Running containers in containers also benefits from user namespace. Docker and other runtimes heavily restrict containerized process capabilities to prevent escapes, and these restrictions prohibit running `docker build` inside a container. Project’s like Jess Frazelle’s [img][img] get around this by using user namespaces to support `Dockerfile` `RUN` commands.

[bwrap]: https://github.com/projectatomic/bubblewrap
[flatpak]: https://flatpak.org/
[img]: https://github.com/genuinetools/img
## User namespace security
User namespaces are notorious for privilege escalation bugs because they expose root capabilities to unprivileged users. This can cause issues with kernel code that doesn’t account for mapped PIDs/GIDs ([CVE-2014-4014][CVE-2014-4014], [CVE-2009-1338][CVE-2009-1338]), or allow bugs that would only be exploitable by root to be reachable by other processes ([CVE-2018-18955][CVE-2018-18955], [CVE-2017-7184][CVE-2017-7184], [CVE-2016-8655][CVE-2016-8655]). Historically, [RHEL][rhel7] and [Debian](https://lwn.net/Articles/673597/) have disabled user namespaces by default and require kernel parameters to use.

[Jess Frazelle noted][jess-user-ns] that the recent runc container escape ([CVE-2019-5736][CVE-2019-5736]) is mitigated by user namespaces. While true, [not running root in containers][liz-no-root] is equally valid and probably more realistic for most architectures.

Like many things in containerland, user namespaces don’t make isolation perfect. The hope is that allowing regular processes to use namespaces for sandboxing outweighs the increased attack surface.

[cloud-config]: https://a16z.com/2019/01/18/notes-on-security-in-2019/
[CVE-2009-1338]: https://nvd.nist.gov/vuln/detail/CVE-2009-1338
[CVE-2014-4014]: https://nvd.nist.gov/vuln/detail/CVE-2014-4014
[CVE-2016-8655]: https://nvd.nist.gov/vuln/detail/CVE-2016-8655
[CVE-2017-7184]: https://nvd.nist.gov/vuln/detail/CVE-2017-7184
[CVE-2018-18955]: https://nvd.nist.gov/vuln/detail/CVE-2018-18955
[CVE-2019-5736]: https://nvd.nist.gov/vuln/detail/CVE-2019-5736
[rhel7]: https://rhelblog.redhat.com/2014/03/31/containers/
[jess-user-ns]: https://twitter.com/jessfraz/status/1095134939161317377
[liz-no-root]: https://twitter.com/lizrice/status/986652996816855045
