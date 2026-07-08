#!/bin/bash
set -e

IMAGE_NAME="${IMAGE_NAME:-ceph:16.2.14-fix}"
BUILD_DIR="${BUILD_DIR:-./build}"
STAGE_DIR="./stage"

echo "=== Packaging Ceph 16.2.14-fix Production Image ==="
echo "Build dir: $BUILD_DIR"
echo "Image name: $IMAGE_NAME"
echo ""

if [ ! -f "$BUILD_DIR/bin/radosgw" ]; then
    echo "ERROR: Build artifacts not found in $BUILD_DIR/bin/"
    echo "Run the build first (build-with-bundled-boost.sh or build-complete.sh)"
    exit 1
fi

echo "Step 1/4: Preparing staging directory..."
rm -rf "$STAGE_DIR"
mkdir -p "$STAGE_DIR/bin" "$STAGE_DIR/lib" "$STAGE_DIR/rados-classes" "$STAGE_DIR/erasure-code"

echo "Step 2/4: Stripping and copying binaries..."
BINARIES=(
    ceph
    ceph-authtool
    ceph-bluestore-tool
    ceph-conf
    ceph-crash
    ceph-dencoder
    ceph-diff-sorted
    ceph-erasure-code-tool
    ceph-fuse
    ceph-immutable-object-cache
    ceph-kvstore-tool
    ceph-mds
    ceph-mgr
    ceph-mon
    ceph-monstore-tool
    ceph-objectstore-tool
    ceph-osd
    ceph-osdomap-tool
    ceph-post-file
    ceph-syn
    cephfs-data-scan
    cephfs-journal-tool
    cephfs-mirror
    cephfs-table-tool
    crushtool
    init-ceph
    monmaptool
    osdmaptool
    rados
    radosgw
    radosgw-admin
    radosgw-es
    radosgw-object-expirer
    radosgw-token
    rbd
    rbd-fuse
    rbd-mirror
    rbd-nbd
)

for bin in "${BINARIES[@]}"; do
    if [ -f "$BUILD_DIR/bin/$bin" ]; then
        cp "$BUILD_DIR/bin/$bin" "$STAGE_DIR/bin/"
        strip --strip-debug "$STAGE_DIR/bin/$bin" 2>/dev/null || true
        echo "  + $bin ($(du -h "$STAGE_DIR/bin/$bin" | cut -f1))"
    else
        echo "  - $bin (not found, skipping)"
    fi
done

echo ""
echo "Step 3/4: Stripping and copying libraries..."

LIBS=(
    libceph-common.so.2
    librados.so.2.0.0
    libradosgw.so.2.0.0
    libradosstriper.so.1.0.0
    librbd.so.1.16.0
    librgw.so.2.0.0
    libcephfs.so.2.0.0
    libcephsqlite.so
    libbluestore_tp.so.1.0.0
    libosd_tp.so.1.0.0
    libos_tp.so.1.0.0
    librados_tp.so.2.0.0
    librbd_tp.so.1.0.0
    librgw_op_tp.so.2.0.0
    librgw_rados_tp.so.2.0.0
)

for lib in "${LIBS[@]}"; do
    if [ -f "$BUILD_DIR/lib/$lib" ]; then
        cp "$BUILD_DIR/lib/$lib" "$STAGE_DIR/lib/"
        strip --strip-debug "$STAGE_DIR/lib/$lib" 2>/dev/null || true
        echo "  + $lib ($(du -h "$STAGE_DIR/lib/$lib" | cut -f1))"
    else
        echo "  - $lib (not found, skipping)"
    fi
done

# Create symlinks for soname versioning
cd "$STAGE_DIR/lib"
ln -sf libceph-common.so.2 libceph-common.so
ln -sf librados.so.2.0.0 librados.so.2
ln -sf librados.so.2.0.0 librados.so
ln -sf libradosgw.so.2.0.0 libradosgw.so.2
ln -sf libradosgw.so.2.0.0 libradosgw.so
ln -sf libradosstriper.so.1.0.0 libradosstriper.so.1
ln -sf libradosstriper.so.1.0.0 libradosstriper.so
ln -sf librbd.so.1.16.0 librbd.so.1
ln -sf librbd.so.1.16.0 librbd.so
ln -sf librgw.so.2.0.0 librgw.so.2
ln -sf librgw.so.2.0.0 librgw.so
ln -sf libcephfs.so.2.0.0 libcephfs.so.2
ln -sf libcephfs.so.2.0.0 libcephfs.so
ln -sf libbluestore_tp.so.1.0.0 libbluestore_tp.so.1
ln -sf libbluestore_tp.so.1.0.0 libbluestore_tp.so
ln -sf libosd_tp.so.1.0.0 libosd_tp.so.1
ln -sf libosd_tp.so.1.0.0 libosd_tp.so
ln -sf libos_tp.so.1.0.0 libos_tp.so.1
ln -sf libos_tp.so.1.0.0 libos_tp.so
ln -sf librados_tp.so.2.0.0 librados_tp.so.2
ln -sf librados_tp.so.2.0.0 librados_tp.so
ln -sf librbd_tp.so.1.0.0 librbd_tp.so.1
ln -sf librbd_tp.so.1.0.0 librbd_tp.so
ln -sf librgw_op_tp.so.2.0.0 librgw_op_tp.so.2
ln -sf librgw_op_tp.so.2.0.0 librgw_op_tp.so
ln -sf librgw_rados_tp.so.2.0.0 librgw_rados_tp.so.2
ln -sf librgw_rados_tp.so.2.0.0 librgw_rados_tp.so
cd - > /dev/null

# Copy OSD class plugins (rados-classes)
echo ""
echo "  Copying OSD class plugins..."
for cls in "$BUILD_DIR"/lib/libcls_*.so.*.0.0; do
    if [ -f "$cls" ]; then
        base=$(basename "$cls")
        cp "$cls" "$STAGE_DIR/rados-classes/"
        strip --strip-debug "$STAGE_DIR/rados-classes/$base" 2>/dev/null || true
        # Create soname symlinks
        soname=$(echo "$base" | sed 's/\.[0-9]*\.[0-9]*$//')
        shortname=$(echo "$base" | sed 's/\.so\..*/\.so/')
        ln -sf "$base" "$STAGE_DIR/rados-classes/$soname"
        ln -sf "$base" "$STAGE_DIR/rados-classes/$shortname"
    fi
done
echo "  $(ls "$STAGE_DIR/rados-classes/"/*.so 2>/dev/null | wc -l) class plugins"

# Copy erasure-code plugins
echo "  Copying erasure-code plugins..."
for ec in "$BUILD_DIR"/lib/libec_*.so; do
    if [ -f "$ec" ]; then
        cp "$ec" "$STAGE_DIR/erasure-code/"
        strip --strip-debug "$STAGE_DIR/erasure-code/$(basename "$ec")" 2>/dev/null || true
    fi
done
echo "  $(ls "$STAGE_DIR/erasure-code/"/*.so 2>/dev/null | wc -l) EC plugins"

# Copy compressor plugins (to /usr/lib64/ceph/compressor/)
echo "  Copying compressor plugins..."
mkdir -p "$STAGE_DIR/ceph-lib/compressor"
for comp in libceph_lz4 libceph_snappy libceph_zlib libceph_zstd; do
    f="$BUILD_DIR/lib/${comp}.so.2.0.0"
    if [ -f "$f" ]; then
        cp "$f" "$STAGE_DIR/ceph-lib/compressor/"
        strip --strip-debug "$STAGE_DIR/ceph-lib/compressor/$(basename "$f")" 2>/dev/null || true
        ln -sf "${comp}.so.2.0.0" "$STAGE_DIR/ceph-lib/compressor/${comp}.so.2"
        ln -sf "${comp}.so.2.0.0" "$STAGE_DIR/ceph-lib/compressor/${comp}.so"
    fi
done
echo "  $(ls "$STAGE_DIR/ceph-lib/compressor/"/*.so 2>/dev/null | wc -l) compressor plugins"

# Copy crypto plugins (to /usr/lib64/ceph/crypto/)
echo "  Copying crypto plugins..."
mkdir -p "$STAGE_DIR/ceph-lib/crypto"
if [ -f "$BUILD_DIR/lib/libceph_crypto_isal.so.1.0.0" ]; then
    cp "$BUILD_DIR/lib/libceph_crypto_isal.so.1.0.0" "$STAGE_DIR/ceph-lib/crypto/"
    strip --strip-debug "$STAGE_DIR/ceph-lib/crypto/libceph_crypto_isal.so.1.0.0" 2>/dev/null || true
    ln -sf libceph_crypto_isal.so.1.0.0 "$STAGE_DIR/ceph-lib/crypto/libceph_crypto_isal.so.1"
    ln -sf libceph_crypto_isal.so.1.0.0 "$STAGE_DIR/ceph-lib/crypto/libceph_crypto_isal.so"
fi
if [ -f "$BUILD_DIR/lib/libceph_crypto_openssl.so" ]; then
    cp "$BUILD_DIR/lib/libceph_crypto_openssl.so" "$STAGE_DIR/ceph-lib/crypto/"
    strip --strip-debug "$STAGE_DIR/ceph-lib/crypto/libceph_crypto_openssl.so" 2>/dev/null || true
fi

# Copy denc modules (to /usr/lib64/ceph/denc/)
echo "  Copying denc modules..."
mkdir -p "$STAGE_DIR/ceph-lib/denc"
for f in "$BUILD_DIR"/lib/denc-mod-*.so; do
    if [ -f "$f" ]; then
        cp "$f" "$STAGE_DIR/ceph-lib/denc/"
        strip --strip-debug "$STAGE_DIR/ceph-lib/denc/$(basename "$f")" 2>/dev/null || true
    fi
done
echo "  $(ls "$STAGE_DIR/ceph-lib/denc/"/*.so 2>/dev/null | wc -l) denc modules"

# Copy librbd plugins (to /usr/lib64/ceph/librbd/)
echo "  Copying librbd plugins..."
mkdir -p "$STAGE_DIR/ceph-lib/librbd"
if [ -f "$BUILD_DIR/lib/libceph_librbd_parent_cache.so.1.0.0" ]; then
    cp "$BUILD_DIR/lib/libceph_librbd_parent_cache.so.1.0.0" "$STAGE_DIR/ceph-lib/librbd/"
    strip --strip-debug "$STAGE_DIR/ceph-lib/librbd/libceph_librbd_parent_cache.so.1.0.0" 2>/dev/null || true
    ln -sf libceph_librbd_parent_cache.so.1.0.0 "$STAGE_DIR/ceph-lib/librbd/libceph_librbd_parent_cache.so.1"
    ln -sf libceph_librbd_parent_cache.so.1.0.0 "$STAGE_DIR/ceph-lib/librbd/libceph_librbd_parent_cache.so"
fi

# Copy Python bindings (rados, rbd, cephfs, rgw)
echo "  Copying Python bindings..."
mkdir -p "$STAGE_DIR/python"
PYMOD_DIR="$BUILD_DIR/lib/cython_modules/lib.3"
for mod in rados rbd cephfs rgw; do
    f=$(ls "$PYMOD_DIR/${mod}".cpython-*.so 2>/dev/null | head -1)
    if [ -n "$f" ]; then
        cp "$f" "$STAGE_DIR/python/"
        strip --strip-debug "$STAGE_DIR/python/$(basename "$f")" 2>/dev/null || true
        echo "  + $(basename "$f") ($(du -h "$STAGE_DIR/python/$(basename "$f")" | cut -f1))"
    else
        echo "  - $mod.so (not found, skipping)"
    fi
done

# Copy Python source modules (ceph CLI needs these)
for pymod in ceph_argparse.py ceph_daemon.py ceph_volume_client.py; do
    if [ -f "src/pybind/$pymod" ]; then
        cp "src/pybind/$pymod" "$STAGE_DIR/python/"
        echo "  + $pymod"
    fi
done

# Copy shell scripts from source
echo "  Copying shell scripts..."
mkdir -p "$STAGE_DIR/scripts"
for script in ceph-run ceph-clsinfo ceph-rbdnamer; do
    if [ -f "src/$script" ]; then
        cp "src/$script" "$STAGE_DIR/scripts/"
        echo "  + $script"
    fi
done
if [ -f "src/tools/cephfs/top/cephfs-top" ]; then
    cp "src/tools/cephfs/top/cephfs-top" "$STAGE_DIR/scripts/"
    echo "  + cephfs-top"
fi

# Create ceph-volume entry point script
mkdir -p "$STAGE_DIR/sbin"
cat > "$STAGE_DIR/sbin/ceph-volume" <<'SCRIPT'
#!/usr/bin/python3
import sys
from ceph_volume.main import Volume
sys.exit(Volume())
SCRIPT
chmod +x "$STAGE_DIR/sbin/ceph-volume"
echo "  + ceph-volume (/usr/sbin/)"

# Create ceph-volume-systemd entry point script
cat > "$STAGE_DIR/sbin/ceph-volume-systemd" <<'SCRIPT'
#!/usr/bin/python3
import sys
from ceph_volume.systemd import main
sys.exit(main())
SCRIPT
chmod +x "$STAGE_DIR/sbin/ceph-volume-systemd"
echo "  + ceph-volume-systemd (/usr/sbin/)"

# Copy ceph-volume Python package
echo "  Copying ceph-volume..."
mkdir -p "$STAGE_DIR/python/ceph_volume"
cp -a src/ceph-volume/ceph_volume/* "$STAGE_DIR/python/ceph_volume/"
find "$STAGE_DIR/python/ceph_volume" -name "__pycache__" -exec rm -rf {} + 2>/dev/null || true
echo "  + ceph_volume/ ($(find "$STAGE_DIR/python/ceph_volume" -name "*.py" | wc -l) files)"

# Copy python-common 'ceph' package (MGR modules depend on this)
echo "  Copying python-common (ceph package)..."
mkdir -p "$STAGE_DIR/python/ceph"
cp -a src/python-common/ceph/* "$STAGE_DIR/python/ceph/"
find "$STAGE_DIR/python/ceph" -name "__pycache__" -exec rm -rf {} + 2>/dev/null || true
rm -rf "$STAGE_DIR/python/ceph/tests"
echo "  + ceph/ ($(find "$STAGE_DIR/python/ceph" -name "*.py" | wc -l) files)"

# Copy MGR Python modules
echo "  Copying MGR modules..."
mkdir -p "$STAGE_DIR/mgr"
for mod in src/pybind/mgr/*/; do
    modname=$(basename "$mod")
    case "$modname" in
        tests|test_orchestrator|wheelhouse|CMakeLists.txt|__pycache__) continue ;;
    esac
    if [ -d "$mod" ]; then
        cp -a "$mod" "$STAGE_DIR/mgr/"
    fi
done
# Copy top-level mgr files
cp src/pybind/mgr/mgr_module.py "$STAGE_DIR/mgr/" 2>/dev/null || true
cp src/pybind/mgr/mgr_util.py "$STAGE_DIR/mgr/" 2>/dev/null || true
find "$STAGE_DIR/mgr" -name "__pycache__" -exec rm -rf {} + 2>/dev/null || true
echo "  $(ls -d "$STAGE_DIR/mgr/"*/ 2>/dev/null | wc -l) MGR modules"

echo ""
echo "Staging complete:"
du -sh "$STAGE_DIR/bin" "$STAGE_DIR/lib" "$STAGE_DIR/rados-classes" "$STAGE_DIR/erasure-code" "$STAGE_DIR/python" "$STAGE_DIR/scripts" "$STAGE_DIR/mgr"
echo "Total: $(du -sh "$STAGE_DIR" | cut -f1)"

echo ""
echo "Step 4/4: Building Docker image..."
docker build -t "$IMAGE_NAME" -f Dockerfile.production .

echo ""
echo "=== Done! ==="
echo "Image: $IMAGE_NAME"
docker images "$IMAGE_NAME"
echo ""
echo "Quick verify:"
echo "  docker run --rm $IMAGE_NAME rgw --version"
echo "  docker run --rm $IMAGE_NAME ceph --version"
echo ""
echo "Run a daemon:"
echo "  docker run -d --name ceph-rgw --network host \\"
echo "    -v /etc/ceph:/etc/ceph:ro \\"
echo "    -v /var/lib/ceph:/var/lib/ceph \\"
echo "    $IMAGE_NAME rgw --name client.rgw.gw0"
