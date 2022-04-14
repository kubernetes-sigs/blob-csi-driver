###############################################################################
# Spec file for blobfuse-proxy
################################################################################
# Configured to be built by  non-root user
################################################################################
#
Summary: Utility scripts for creating RPM package for blobfuse-proxy
Name: blobfuse-proxy
Version: v0.1.0
Release: 1
License: Apache
Group: System
Packager: David Both
Requires: bash
BuildRoot: ~/rpmbuild/
Source0: blobfuse-proxy

%description
Utility scripts for creating RPM package for blobfuse-proxy

%install
mkdir -p %{buildroot}/usr/bin/
cp %{SOURCE0} %{buildroot}/usr/bin/blobfuse-proxy

%files
/usr/bin/blobfuse-proxy
