

# How to produce a custom Linux OS image

A LCOW custom Linux OS image was divided into two parts: a Linux kernel module and a set of user-mode components. Both parts were highly customized for the purpose of supporting Linux Hyper-V container on Windows


## How to build custom kernel module

- Have your 4.11 kernel source tree ready

- Apply additional [4.11 patches](../kernel/patches_readme.md) to your 4.11 kernel source tree

- Use the recommended [Kconfig](../kernel/kernel_config-4.11.x) to include all LCOW necessary kernel components

- Build your kernel 


    Note:  The key delta between the upsteam default setting and above kconfig is in the area of ACPI/NIFT/NVDIMM/OverlyFS/9pFS/Vsock/HyerpV settings, which were set to be built-in instead of modules.
           The Kconfig above is still a work in process in terms of trimming any unnecessary components from the kernel image.  

## How to construct user-mode components

The expected user mode directory structure is required to constructed as follows: [See the complete user-mode file list](../kernel/completeUsermodeFileLists.md)

Under the / directory, it should have the following subdirectories:

- /tmp 
- /proc 
- /bin 
- /dev 
- /run 
- /etc 
- /usr 
- /mnt 
- /sys    

- /init 
- /root 
- /sbin 
- /lib64 
- /lib      

Here are the expected contents of each subdirectory /file
     
1. Subdirectories with **empty** contents:  /tmp /proc /dev /run /etc /usr /mnt /sys 

2. **/init** 
   This is the [init script file](../kernel/scripts/init_script)

3. **/root** : this is the home directory of the root account. 

4. **/sbin** : 
    - /sbin/runc  

              Note:this is the "runc" binary for hosting the container execution environment. 
              It needs to be the following release
              runc version 1.0.0-rc3
              commit: 992a5be178a62e026f4069f443c6164912adbf09
              spec: 1.0.0-rc5

    - /sbin/[udhcpc_config.script](https://github.com/mirror/busybox/blob/master/examples/udhcp/simple.script)
    
5. **/lib64** :

       /lib64/ld-linux-x86-64.so.2

6. **/lib** : 

       /lib/x86_64-linux-gnu
       /lib/x86_64-linux-gnu/libe2p.so.2
       /lib/x86_64-linux-gnu/libcom_err.so.2
       /lib/x86_64-linux-gnu/libc.so.6
       /lib/x86_64-linux-gnu/libdl.so.2
       /lib/x86_64-linux-gnu/libapparmor.so.1
       /lib/x86_64-linux-gnu/libseccomp.so.2
       /lib/x86_64-linux-gnu/libblkid.so.1
       /lib/x86_64-linux-gnu/libpthread.so.0
       /lib/x86_64-linux-gnu/libext2fs.so.2
       /lib/x86_64-linux-gnu/libuuid.so.1
       /lib/modules

7. **/bin** : binaries in this subdir are categorised into four groups
        
    - [GCS binaries](gcsbuildinstructions.md)

            /bin/exportSandbox
            /bin/gcs
            /bin/gcstools
            /bin/netnscfg
            /bin/remotefs
            /bin/tar2vhd
            /bin/vhd2tar

            Note : exportSandbox, vhd2tar, tar2vhd, remotefs, and netnscfg are actually hard links to the "gcstools' file

    - Required binaires: utilities used by gcs

             /bin/sh
             /bin/mkfs.ext4
             /bin/blockdev
             /bin/mkdir
             /bin/rmdir
             /bin/mount
             /bin/udhcpd
             /bin/ip
             /bin/iproute
             /bin/hostname

    - Required binaires: utilities used by docker

             /bin/ls
             /bin/cat
             /bin/test

    - Debugging tools: mostly from busybox tool set
       

# Supported LCOW custom Linux OS packaging formats

A LCOW custom Linux OS could be packaged into two different supported formats: 
- Kernel + Initrd: vmlinuz and initrd.img
- VHD: a VHDx file



