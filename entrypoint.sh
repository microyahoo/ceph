#!/bin/bash
set -e

CEPH_DAEMON=${1:-${CEPH_DAEMON}}

case "$CEPH_DAEMON" in
    mon|ceph-mon)
        shift || true
        exec /usr/bin/ceph-mon -f "$@"
        ;;
    osd|ceph-osd)
        shift || true
        exec /usr/bin/ceph-osd -f "$@"
        ;;
    mds|ceph-mds)
        shift || true
        exec /usr/bin/ceph-mds -f "$@"
        ;;
    rgw|radosgw)
        shift || true
        exec /usr/bin/radosgw -f "$@"
        ;;
    radosgw-admin)
        shift || true
        exec /usr/bin/radosgw-admin "$@"
        ;;
    mgr|ceph-mgr)
        shift || true
        exec /usr/bin/ceph-mgr -f "$@"
        ;;
    rados)
        shift || true
        exec /usr/bin/rados "$@"
        ;;
    ceph)
        shift || true
        exec /usr/bin/ceph "$@"
        ;;
    bash|sh)
        exec /bin/bash
        ;;
    *)
        if [ -n "$CEPH_DAEMON" ] && [ -x "/usr/bin/$CEPH_DAEMON" ]; then
            exec "/usr/bin/$CEPH_DAEMON" "$@"
        elif [ -n "$CEPH_DAEMON" ] && [ -x "/usr/bin/ceph-$CEPH_DAEMON" ]; then
            exec "/usr/bin/ceph-$CEPH_DAEMON" "$@"
        fi
        echo "Usage: docker run ceph:<tag> <daemon> [options]"
        echo ""
        echo "Available daemons:"
        echo "  mon, osd, mds, mgr, rgw, radosgw-admin, rados, ceph"
        echo ""
        echo "Examples:"
        echo "  docker run ceph:16.2.14-fix rgw --name client.rgw.gw0"
        echo "  docker run ceph:16.2.14-fix mon --id node1"
        echo "  docker run ceph:16.2.14-fix osd --id 0"
        exit 1
        ;;
esac
