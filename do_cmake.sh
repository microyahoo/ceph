#!/usr/bin/env bash
set -ex

if [ -d .git ]; then
    git submodule update --init --recursive --progress
fi

: ${BUILD_DIR:=build}
: ${CEPH_GIT_DIR:=..}

PYBUILD="3"
# ARGS="-GNinja"
if [ -r /etc/os-release ]; then
  source /etc/os-release
  case "$ID" in
      fedora)
          if [ "$VERSION_ID" -ge "39" ] ; then
            PYBUILD="3.12"
          else
            # Fedora 37 and above
            PYBUILD="3.11"
          fi
          ;;
      almalinux|rocky|rhel|centos)
          MAJOR_VER=$(echo "$VERSION_ID" | sed -e 's/\..*$//')
          if [ "$MAJOR_VER" -ge "9" ] ; then
              PYBUILD="3.9"
          elif [ "$MAJOR_VER" -ge "8" ] ; then
              PYBUILD="3.6"
          fi
          ;;
      opensuse*|suse|sles)
          PYBUILD="3"
          ARGS+=" -DWITH_RADOSGW_AMQP_ENDPOINT=OFF"
          ARGS+=" -DWITH_RADOSGW_KAFKA_ENDPOINT=OFF"
          ;;
      ubuntu)
          MAJOR_VER=$(echo "$VERSION_ID" | sed -e 's/\..*$//')
          if [ "$MAJOR_VER" -ge "22" ] ; then
              PYBUILD="3.10"
          fi
          ;;

  esac
elif [ "$(uname)" == FreeBSD ] ; then
  PYBUILD="3"
  ARGS+=" -DWITH_RADOSGW_AMQP_ENDPOINT=OFF"
  ARGS+=" -DWITH_RADOSGW_KAFKA_ENDPOINT=OFF"
else
  echo Unknown release
  exit 1
fi

ARGS+=" -DWITH_PYTHON3=${PYBUILD}"

if type ccache > /dev/null 2>&1 ; then
    echo "enabling ccache"
    ARGS+=" -DWITH_CCACHE=ON"
fi

cxx_compiler="g++"
c_compiler="gcc"
# 20 is used for more future-proof
for i in $(seq 20 -1 11); do
  if type -t gcc-$i > /dev/null; then
    cxx_compiler="g++-$i"
    c_compiler="gcc-$i"
    break
  fi
done
ARGS+=" -DCMAKE_CXX_COMPILER=$cxx_compiler"
ARGS+=" -DCMAKE_C_COMPILER=$c_compiler"
#
if type cmake3 > /dev/null 2>&1 ; then
    CMAKE=cmake3
else
    CMAKE=cmake
fi
ARGS="-DCMAKE_BUILD_TYPE=Debug -DWITH_SEASTAR=OFF -DWITH_MGR_DASHBOARD_FRONTEND=OFF -DENABLE_GIT_VERSION=OFF -DWITH_TESTS=OFF -DWITH_CCACHE=ON -DWITH_LTTNG=OFF -DWITH_RDMA=OFF -DWITH_FUSE=OFF -DWITH_DPDK=OFF $ARGS"
ARGS="-DCMAKE_EXPORT_COMPILE_COMMANDS=1 -DWITH_RADOSGW_AMQP_ENDPOINT=OFF -DWITH_RADOSGW_KAFKA_ENDPOINT=OFF $ARGS"

if [ -e $BUILD_DIR ]; then
    git submodule update --init --recursive
else
    mkdir $BUILD_DIR
fi
cd $BUILD_DIR
${CMAKE} -DCMAKE_C_FLAGS="-O0 -g3 -gdwarf-4" -DCMAKE_CXX_FLAGS="-O0 -g3 -gdwarf-4" $ARGS "$@" $CEPH_GIT_DIR || exit 1 
set +x

# minimal config to find plugins
cat <<EOF > ceph.conf
[global]
plugin dir = lib
erasure code dir = lib
EOF

echo done.

if [[ ! "$ARGS $@" =~ "-DCMAKE_BUILD_TYPE" ]]; then
  cat <<EOF

****
WARNING: do_cmake.sh now creates debug builds by default. Performance
may be severely affected. Please use -DCMAKE_BUILD_TYPE=RelWithDebInfo
if a performance sensitive build is required.
****
EOF
fi

