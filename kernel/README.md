# Custom Linux kernel for LCOW

Here you will find the steps to build a custom kernel for the
Linux Hyper-V container on Windows (**LCOW**). To build the full image,
please follow the instruction from [how to produce a custom Linux OS
image](../docs/customosbuildinstructions.md)

## Patches

So far **LCOW** is based on Linux Kernel 4.11, you can download the Linux source
code from [kernel.org](https://cdn.kernel.org/pub/linux/kernel/v4.x/linux-4.11.tar.xz).

Once you get the _4.11 kernel_, apply all the patches files located in the
[patches-4.11.x](./patches-4.11.x) directory. You should be in the Linux kernel
source directory

```
patch -p1 < /path/to/kernel/patches-4.11.x/0001-*
patch -p1 < /path/to/kernel/patches-4.11.x/0002-*
```

or in a simple line

```
for p in /path/to/kernel/patches-4.11.x/*.patch; do patch -p1 < $p;  done
```

Beside the patches located in the [patches-4.11.x](./patches-4.11.x) directory,
you need to apply a set of patches to enable the **Hyper-V vsock transport**
feature in the Linux kernel. Please refer to the following section to view the
instructions to get them.

#### Instructions for getting Hyper-V vsock patch set

These patches enables the **Hyper-V vsock transport** feature,
this instructions is to get them from a developer repository and
assuming you have a _Linux GIT repository_  already

```
git config --global user.name "yourname"
git config --global user.email youremailaddress 
 
git remote add -f dexuan-github https://github.com/dcui/linux.git
 
git cherry-pick c248b14174e1337c1461f9b13a573ad90a136e1c
git cherry-pick 008d8d8bc0c86473a8549a365bee9a479243e412
git cherry-pick 4713066c11b2396eafd2873cbed7bdd72d1571eb
git cherry-pick 1df677b35ff010d0def33f5420773015815cf843
git cherry-pick 3476be340d2ff777609fca3e763da0292acbfc45
git cherry-pick b5566b1b6e5cb19b381590587f841f950caabe4d
git cherry-pick 6f1aa69011356ff95ed6c57400095e5f2d9eb900
git cherry-pick 2fac74605d2db862caaaf4890239b57095fba832
git cherry-pick 2e307800c6a01cd789afe34eccbcabf384959b3f
git cherry-pick 83c8635b893bbc0b5b329c632cea0382d5479763
git cherry-pick a2c08e77b8ceb1f146cdc5136e85e7a4c2c9b7cb
git cherry-pick be1ce15dfbdfe3f42c8ed23c5904674d5d90b545
git cherry-pick 8457502df9dd379ddbdfa42a8c9a6421bb3482f1
git cherry-pick 1b91aa6d0e745d9765e3d90058928829f0b0bd40
git cherry-pick 531389d1dc73e2be3ee5dbf2091b6f5e74d9764c
git cherry-pick c49aced6328557e6c1f5cf6f58e1fae96fb58fa0
git cherry-pick 651dae7de6c6f066c08845ec7335bfb231d5eabe
```

Another way to get the patches is to download them from the following list and
apply them in the same order:

1.  https://github.com/dcui/linux/commit/c248b14174e1337c1461f9b13a573ad90a136e1c.patch
2.  https://github.com/dcui/linux/commit/008d8d8bc0c86473a8549a365bee9a479243e412.patch
3.  https://github.com/dcui/linux/commit/4713066c11b2396eafd2873cbed7bdd72d1571eb.patch
4.  https://github.com/dcui/linux/commit/1df677b35ff010d0def33f5420773015815cf843.patch
5.  https://github.com/dcui/linux/commit/3476be340d2ff777609fca3e763da0292acbfc45.patch
6.  https://github.com/dcui/linux/commit/b5566b1b6e5cb19b381590587f841f950caabe4d.patch
7.  https://github.com/dcui/linux/commit/6f1aa69011356ff95ed6c57400095e5f2d9eb900.patch
8.  https://github.com/dcui/linux/commit/2fac74605d2db862caaaf4890239b57095fba832.patch
9.  https://github.com/dcui/linux/commit/2e307800c6a01cd789afe34eccbcabf384959b3f.patch
10. https://github.com/dcui/linux/commit/83c8635b893bbc0b5b329c632cea0382d5479763.patch
11. https://github.com/dcui/linux/commit/a2c08e77b8ceb1f146cdc5136e85e7a4c2c9b7cb.patch
12. https://github.com/dcui/linux/commit/be1ce15dfbdfe3f42c8ed23c5904674d5d90b545.patch
13. https://github.com/dcui/linux/commit/8457502df9dd379ddbdfa42a8c9a6421bb3482f1.patch
14. https://github.com/dcui/linux/commit/1b91aa6d0e745d9765e3d90058928829f0b0bd40.patch
15. https://github.com/dcui/linux/commit/531389d1dc73e2be3ee5dbf2091b6f5e74d9764c.patch
16. https://github.com/dcui/linux/commit/c49aced6328557e6c1f5cf6f58e1fae96fb58fa0.patch
17. https://github.com/dcui/linux/commit/651dae7de6c6f066c08845ec7335bfb231d5eabe.patch

### Patches structure

In the [patches-4.11.x](./patches-4.11.x) directory you will find the
following patches:

  - [9pfs: added vsock transport support](./patches-4.11.x/0001-Added-vsock-transport-support-to-9pfs.patch)

  - [nvdimm: Lower minimum PMEM size](./patches-4.11.x/0002-NVDIMM-reducded-ND_MIN_NAMESPACE_SIZE-from-4MB-to-4K.patch)
