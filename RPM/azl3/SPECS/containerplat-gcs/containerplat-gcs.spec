# placeholder value for the build version. Replace with real version.
%global build_version @@@BUILD_VERSION_HERE@@@
%global debug_package %{nil}

Summary: Build and install GCS components from hcsshim
Name: containerplat-gcs
Version: %{build_version}
Release: 1%{?dist}
License: MIT
URL: NONE
Source: %{name}-%{build_version}.tar.gz
BuildRequires: bash
BuildRequires: tar
BuildRequires: kmod
BuildRequires: kmod-devel
BuildRequires: golang
BuildRequires: gcc
BuildRequires: git
Requires: kmod
Requires: glibc
Requires: chrony
Requires: moby-runc
Requires: e2fsprogs

%description
Build and install GCS components from hcsshim

%package dev
Summary: Package with additional test tools.
Requires: %{name} = %{version}-%{release}
Provides: %{name}-dev = %{version}-%{release}

%description dev
This package contains additional utilities for testing.

%prep
%setup -q -n %{name}-%{version}

%build
make clean
# golang requirement **should** be the Microsoft Go distribution
make MSGO=1 KMOD=1 GO_FLAGS_EXTRA=-trimpath out/delta.tar.gz out/delta-dev.tar.gz

%install
mkdir -p %{buildroot}%{_bindir}
mkdir -p %{buildroot}/info/

mkdir -p delta
tar -xvf out/delta.tar.gz -C delta
install -m 755 -p delta/bin/gcs %{buildroot}%{_bindir}/gcs
install -m 755 -p delta/bin/gcstools %{buildroot}%{_bindir}/gcstools
install -m 755 -p delta/bin/generichook %{buildroot}%{_bindir}/generichook
install -m 755 -p delta/bin/install-drivers %{buildroot}%{_bindir}/install-drivers
install -m 755 -p delta/bin/vsockexec %{buildroot}%{_bindir}/vsockexec
install -m 755 -p delta/bin/wait-paths %{buildroot}%{_bindir}/wait-paths
install -m 755 -p delta/info/gcs.branch %{buildroot}/info/gcs.branch
install -m 755 -p delta/info/gcs.commit %{buildroot}/info/gcs.commit
install -m 755 -p delta/init %{buildroot}/init

mkdir delta-dev
tar -xvf out/delta-dev.tar.gz -C delta-dev
install -m 755 -p delta-dev/bin/snp-report %{buildroot}%{_bindir}/snp-report

%files
%{_bindir}/gcs
%{_bindir}/gcstools
%{_bindir}/generichook
%{_bindir}/install-drivers
%{_bindir}/vsockexec
%{_bindir}/wait-paths
/info/gcs.branch
/info/gcs.commit
/init

%files dev
%{_bindir}/gcs
%{_bindir}/gcstools
%{_bindir}/generichook
%{_bindir}/install-drivers
%{_bindir}/vsockexec
%{_bindir}/wait-paths
%{_bindir}/snp-report
/info/gcs.branch
/info/gcs.commit
/init
