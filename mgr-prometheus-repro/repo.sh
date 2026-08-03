# 读写的同时变更集群的状态
while true; do ceph osd set noout; sleep 1; ceph osd unset noout; sleep 1; done

# ./mgr-prometheus-repro \-endpoint http://localhost:9283/metrics       -workers 64       -duration 10000m       -timeout 10s
