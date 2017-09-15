# How to build 4.12.x based custom Linux kernel for LCOW

You can download the Linux 4.12 source code from [kernel.org](https://cdn.kernel.org/pub/linux/kernel/v4.x/linux-4.12.tar.xz).

Once you get the _4.12 kernel_, apply all the following patches 

## 1. Patch for "nvdimm: Lower minimum PMEM size"
 
This [patch file](./0002-NVDIMM-reducded-ND_MIN_NAMESPACE_SIZE-from-4MB-to-4K.patch) could be find in this directory.  

You should be in the Linux kernel source directory before applying the patch with the following command

```
patch -p1 < /path/to/kernel/patches-4.12.x/0002-*
```


## 2. Patch set for the Hyper-V vsock support

These patches enables the **Hyper-V vsock transport** feature,
this instructions is to get them from a developer repository and
assuming you have a _Linux GIT repository_  already

```
git config --global user.name "yourname"
git config --global user.email youremailaddress 
 
git remote add -f dexuan-github https://github.com/dcui/linux.git
 
git cherry-pick -x 5181302de497cb7d5de37bbc84e01eca676f20d8
git cherry-pick -x b54a12c4e3f18cd48314fd3851f5651446b0e6ee
git cherry-pick -x 866488f04fc4d8ff513697db2f80263e90277291
git cherry-pick -x fdd8e16c855a6c7238c654d7217dcf51c5533307
git cherry-pick -x b02ea409f1fceeaac6fd971db5d095ecc903de2d
git cherry-pick -x 27e512021e36c67dd1c773a52b23d71896c80602
git cherry-pick -x e2c1d1b8e8d17cc9b423688d59ad486c5f38deca
git cherry-pick -x e015b0a767dcab79b8b8361516f3f4322cdc90a7
git cherry-pick -x b9cc90e62104bd001b05d897f84cb7d30d1780bb
git cherry-pick -x 022c888e809721a67ecd3072e6331cbdaab45536
git cherry-pick -x 81304747d9bcba135c9a9d534f3a3190bca92339
git cherry-pick -x db40d92a09ff6b84b6c47e96d0a8d1cb1f83cd36
git cherry-pick -x 0465d97030768485eec5a69a98963e3da7402826
git cherry-pick -x 7592de58cbf8d199d721503385c20a02743425a9
git cherry-pick -x 02d07a9dcdb042f33248fd3aeb1e5c2eca6d3d49
git cherry-pick -x f315dfcf9c3b4b32f43a21664762cbacd8f05d6a
git cherry-pick -x d6f7158fdbac10f9935a506451e3d54d2d50a7c7

```

Another way to get the patches is to download them from the following list and
apply them in the same order:

1.  https://github.com/dcui/linux/commit/5181302de497cb7d5de37bbc84e01eca676f20d8.patch
2.  https://github.com/dcui/linux/commit/b54a12c4e3f18cd48314fd3851f5651446b0e6ee.patch
3.  https://github.com/dcui/linux/commit/866488f04fc4d8ff513697db2f80263e90277291.patch
4.  https://github.com/dcui/linux/commit/fdd8e16c855a6c7238c654d7217dcf51c5533307.patch
5.  https://github.com/dcui/linux/commit/b02ea409f1fceeaac6fd971db5d095ecc903de2d.patch
6.  https://github.com/dcui/linux/commit/27e512021e36c67dd1c773a52b23d71896c80602.patch
7.  https://github.com/dcui/linux/commit/e2c1d1b8e8d17cc9b423688d59ad486c5f38deca.patch
8.  https://github.com/dcui/linux/commit/e015b0a767dcab79b8b8361516f3f4322cdc90a7.patch
9.  https://github.com/dcui/linux/commit/b9cc90e62104bd001b05d897f84cb7d30d1780bb.patch
10. https://github.com/dcui/linux/commit/022c888e809721a67ecd3072e6331cbdaab45536.patch
11. https://github.com/dcui/linux/commit/81304747d9bcba135c9a9d534f3a3190bca92339.patch
12. https://github.com/dcui/linux/commit/db40d92a09ff6b84b6c47e96d0a8d1cb1f83cd36.patch
13. https://github.com/dcui/linux/commit/0465d97030768485eec5a69a98963e3da7402826.patch
14. https://github.com/dcui/linux/commit/7592de58cbf8d199d721503385c20a02743425a9.patch
15. https://github.com/dcui/linux/commit/02d07a9dcdb042f33248fd3aeb1e5c2eca6d3d49.patch
16. https://github.com/dcui/linux/commit/f315dfcf9c3b4b32f43a21664762cbacd8f05d6a.patch
17. https://github.com/dcui/linux/commit/d6f7158fdbac10f9935a506451e3d54d2d50a7c7.patch


## 3. Patch for "ext4: fix fault handling when mounted with -o dax,ro"

There was a regression (see [this 4.11.1 commit](https://git.kernel.org/pub/scm/linux/kernel/git/stable/linux-stable.git/commit/?h=linux-4.11.y&id=5a3651b4a92cbc5230d67d2ce87fb3f7373c7665))
on the readonly DAX enabled readonly ext4 support. 

If you are building kernel >= 4.11.1, you would need to pick up the following fix for this regression to be able to use PMEM devices for Docker layers.

git fetch [fd96b8da68d32a9403726db09b229f4b5ac849c7](https://github.com/torvalds/linux/commit/fd96b8da68d32a9403726db09b229f4b5ac849c7#diff-f959e50cbd17809e773ef7b89a38d3ca)
