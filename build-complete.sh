#!/bin/bash
set -e

echo "=== Building Complete Ceph 16.2.14-fix (All Components) ==="
echo "Source: $(pwd)"
echo "Branch: $(git branch --show-current)"
echo ""
echo "This will build all Ceph components (mon, osd, mds, mgr, rgw, etc.)"
echo "similar to official Ceph builds."
echo ""

# Use Docker to build in CentOS Stream 8 environment
docker build -t ceph-builder:complete -f- . <<'DOCKERFILE'
FROM quay.io/centos/centos:stream8

# Fix CentOS 8 repos (now EOL, moved to vault)
RUN sed -i 's|^mirrorlist=|#mirrorlist=|g' /etc/yum.repos.d/CentOS-*.repo && \
    sed -i 's|^#baseurl=http://mirror.centos.org|baseurl=http://vault.centos.org|g' /etc/yum.repos.d/CentOS-*.repo && \
    dnf clean all

# Install EPEL
RUN dnf install -y epel-release dnf-plugins-core && \
    dnf config-manager --set-enabled powertools && \
    dnf clean all

# Install build tools
RUN dnf install -y \
        git cmake ninja-build \
        gcc gcc-c++ \
        make which wget curl \
        python3 python3-devel python3-pip \
        python3-setuptools python3-Cython \
        rpm-build redhat-rpm-config && \
    dnf clean all

# Install all Ceph dependencies (complete build)
RUN dnf install -y \
        cryptsetup-devel \
        expat-devel \
        fmt-devel \
        fuse-devel \
        gperftools-devel \
        keyutils-libs-devel \
        leveldb-devel \
        libaio-devel \
        libatomic \
        libblkid-devel \
        libcap-ng-devel \
        libcurl-devel \
        libedit-devel \
        libibverbs-devel \
        libicu-devel \
        libnl3-devel \
        liboath-devel \
        librdmacm-devel \
        libtool \
        libuuid-devel \
        libxml2-devel \
        lua-devel \
        lz4-devel \
        ncurses-devel \
        nss-devel \
        openldap-devel \
        openssl-devel \
        snappy-devel \
        sqlite-devel \
        systemd-devel \
        xz-devel \
        zlib-devel \
        python3-sphinx \
        python3-prettytable \
        python3-bcrypt \
        libudev-devel && \
    dnf clean all

WORKDIR /build
DOCKERFILE

echo "Step 1/2: Building all Ceph components with bundled Boost..."
docker run --rm \
    -v "$(pwd):/src" \
    -v "$(pwd)/build:/build" \
    ceph-builder:complete \
    bash -c "
        set -ex
        cd /src

        # Run install-deps.sh to get Python dependencies
        export FOR_MAKE_CHECK=
        ./install-deps.sh || true

        cd /build

        # Configure complete Ceph build with bundled Boost
        # This matches official builds more closely
        cmake /src \
            -GNinja \
            -DCMAKE_BUILD_TYPE=RelWithDebInfo \
            -DCMAKE_INSTALL_PREFIX=/usr \
            -DWITH_TESTS=OFF \
            -DWITH_MANPAGE=OFF \
            -DWITH_SYSTEM_BOOST=OFF \
            -DWITH_SYSTEM_NPM=OFF \
            -DWITH_PYTHON3=3 \
            -DWITH_MGR=ON \
            -DWITH_MGR_DASHBOARD_FRONTEND=OFF \
            -DWITH_RADOSGW=ON \
            -DWITH_RADOSGW_BEAST_FRONTEND=ON \
            -DWITH_RADOSGW_BEAST_OPENSSL=ON

        # Build everything
        # First build Boost (bundled, external project)
        echo '=== Building bundled Boost (this takes ~10 minutes) ==='
        ninja Boost

        echo '=== Starting ninja build (this will take 30-60 minutes) ==='
        ninja -j\$(nproc)

        echo '=== Build completed successfully! ==='
        echo 'Key binaries:'
        ls -lh /build/bin/ceph-mon /build/bin/ceph-osd /build/bin/ceph-mds /build/bin/radosgw /build/bin/ceph-mgr 2>/dev/null || true
    "

echo ""
echo "Step 2/2: Checking build artifacts..."
echo "=== Built binaries ==="
ls -lh build/bin/ceph* build/bin/rados* 2>/dev/null | head -20

if [ -f build/bin/radosgw ]; then
    echo ""
    echo "✓ Complete Ceph build successful!"
    echo "  - radosgw (RGW gateway)"
    echo "  - ceph-mon (Monitor)"
    echo "  - ceph-osd (Object Storage Daemon)"
    echo "  - ceph-mds (Metadata Server)"
    echo "  - ceph-mgr (Manager)"
    echo ""
    echo "All binaries are in ./build/bin/"
else
    echo "✗ Build failed"
    exit 1
fi
