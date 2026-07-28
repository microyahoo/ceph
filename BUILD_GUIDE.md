# Ceph 16.2.14-fix4 编译与打包完整文档

## 概述

本文档记录了从源代码编译 Ceph 16.2.14（含多项 bug 修复）并打包为生产可用 Docker 镜像的完整流程。

**目标分支**: `16.2.14-fix4`
**基准 Tag**: `238ba602515`（16.2.14 release）
**顶部 Commit**: `6491bf5153c` — build: optimize builder image with install-deps.sh and document env variables
**最终产物**: Docker 镜像 `ceph:16.2.14-fix4`（~779MB），包含完整 Ceph 组件（含 ceph-mgr）

---

## 前置条件

- Docker 环境（用于构建和运行）
- 至少 16GB 内存（编译需要）
- 至少 30GB 磁盘空间（源码 + 编译产物）
- 网络访问（拉取 CentOS 8 base image 和 yum 包）

---

## 整体流程（一键构建）

```bash
# 一键完成编译 + 打包
./build-and-package.sh

# 或分步执行
REBUILD_BUILDER=1 ./build-complete.sh   # 编译
./package-production.sh                  # 打包
```

```
源代码 ──→ 容器内编译 ──→ build/ 产物 ──→ strip + stage ──→ 生产镜像
         (CentOS 8)      (20GB未strip)   (strip+MGR模块)    (~779MB)
```

分为两大步骤：
1. **编译**：在 CentOS Stream 8 容器内，使用 bundled Boost 编译完整 Ceph
2. **打包**：strip 二进制，组装精简运行时镜像

---

## 第一步：编译（build-complete.sh）

### 编译环境

- 基础镜像：`quay.io/centos/centos:stream8`
- 编译器：GCC (CentOS 8 默认)
- 构建系统：CMake + Ninja
- Boost：使用 Ceph 自带的 bundled Boost 1.72（非系统 Boost）

### 关键 CMake 配置

```cmake
cmake /src \
    -GNinja \
    -DCMAKE_BUILD_TYPE=RelWithDebInfo \
    -DCMAKE_INSTALL_PREFIX=/usr \
    -DWITH_TESTS=OFF \
    -DWITH_MANPAGE=OFF \
    -DWITH_SYSTEM_BOOST=OFF \        # 关键：使用 bundled Boost
    -DWITH_SYSTEM_NPM=OFF \
    -DWITH_PYTHON3=3 \
    -DWITH_MGR=ON \                  # 启用 ceph-mgr
    -DWITH_MGR_DASHBOARD_FRONTEND=OFF \
    -DWITH_RADOSGW=ON \
    -DWITH_RADOSGW_BEAST_FRONTEND=ON \
    -DWITH_RADOSGW_BEAST_OPENSSL=ON
```

**为什么用 bundled Boost**：CentOS 8 系统只有 Boost 1.66，Ceph Pacific 要求 1.72+。使用 `-DWITH_SYSTEM_BOOST=OFF` 让 Ceph 编译自带的 Boost 源码，这是官方构建方式，避免修改源代码去适配低版本 Boost。

### 源代码修改

相对于官方 16.2.14 tag（`238ba602515`），本分支包含 **18 个 commit**：

| Commit | 说明 |
|--------|------|
| `2b7ec0f08bf` | ceph-volume: allow removable devices but exclude USB |
| `3c8b78304a7` | ceph-volume: fix a bug in _check_generic_reject_reasons |
| `2f6fcbfcd23` | rgw: beast frontend checks for local_endpoint() errors |
| `6b8fa999d35` | rgw: fix use-after-free in concurrent multi-object delete |
| `b186cc164bf` | build: add container build and production image packaging scripts |
| `3822dd011c2` | test: add RGW multi-object delete UAF reproduction tool |
| `424af9a97f1` | options/global: raise bluestore_compression_min_blob_size_* to 64K |
| `f93910ef80c` | os/bluestore: track compression_*blob_size* parameters for online update |
| `514a69a5f2c` | common/options: enable LZ4 compression for BlueStore RocksDB |
| `75ef9c03142` | rgw/beast: use cancel+shutdown instead of close during frontend shutdown |
| `dd66182723f` | build: use --network=host for docker build and run |
| `0598ac17c8b` | mgr/prometheus: prune stale health checks, compress output |
| `ccbed536d3a` | mgr/TTLCache: fix PyObject* lifetime management and cleanup logic |
| `b939a8704ae` | mgr/prometheus: Use RLock to fix deadlock in HealthHistory |
| `cbb1585931e` | mgr/prometheus: metrics header for standby module |
| `7e505c69617` | mgr/prometheus/test_module: Adding unit-test for new classes |
| `e1a08236508` | mgr/prometheus: fix Python 3.6 compatibility for Pacific backport |
| `6491bf5153c` | build: optimize builder image with install-deps.sh and document env variables |

另外为了在外部网络构建，注释了 `install-deps.sh` 中的 Ceph 内部 sepia 仓库配置（第 431-436 行）。

### 运行编译

```bash
# 首次构建（或依赖变更后重建 builder 镜像）
REBUILD_BUILDER=1 ./build-complete.sh

# 后续增量编译（复用已有 builder 镜像）
./build-complete.sh
```

**环境变量**：
| 变量 | 说明 |
|------|------|
| `REBUILD_BUILDER=1` | 强制重建 builder 镜像（添加新依赖后需要） |
| `IMAGE_NAME` | 生产镜像名称（默认 `ceph:16.2.14-fix4`） |

**Builder 镜像构建流程**：
1. `docker build` 安装基础 dnf 包（快速覆盖主要编译依赖）
2. `docker run` + `docker commit` 执行 `install-deps.sh`，将所有剩余依赖（Python 包、条件依赖等）固化到镜像中
3. 后续编译直接复用 `ceph-builder:complete` 镜像，跳过所有依赖安装

**耗时预估**：
| 阶段 | 时间 |
|------|------|
| Builder 镜像构建（首次） | 3-5 分钟 |
| install-deps.sh（首次） | 5-10 分钟 |
| CMake 配置 | 1-2 分钟 |
| Bundled Boost 编译 | 10-15 分钟 |
| Ceph 编译 | 30-60 分钟 |
| **总计（首次）** | **约 50-90 分钟** |
| **总计（增量编译）** | **约 5-30 分钟** |

### 编译产物

编译完成后，产物在 `./build/` 目录：

```
build/
├── bin/           # 二进制文件（7.2GB 未 strip）
│   ├── ceph-mon
│   ├── ceph-osd
│   ├── ceph-mds
│   ├── radosgw
│   ├── radosgw-admin
│   ├── rados
│   ├── rbd
│   └── ...
└── lib/           # 共享库（13GB 未 strip）
    ├── libceph-common.so.2
    ├── librados.so.2.0.0
    ├── libradosgw.so.2.0.0
    ├── libcls_*.so        # OSD class 插件
    ├── libec_*.so         # Erasure-code 插件
    └── cython_modules/    # Python 绑定
```

---

## 第二步：打包生产镜像（package-production.sh）

### 打包流程

```bash
./package-production.sh
```

脚本执行 4 个步骤：

#### Step 1: 准备 staging 目录

清理并创建 `./stage/` 目录结构。

#### Step 2: Strip 并复制二进制

将以下核心二进制 strip debug info 后复制到 `stage/bin/`：

| 二进制 | Strip 后大小 | 说明 |
|--------|-------------|------|
| ceph | 48K | CLI 工具（Python 脚本） |
| ceph-mon | 12M | Monitor 守护进程 |
| ceph-osd | 30M | OSD 守护进程 |
| ceph-mds | 8.6M | MDS 守护进程 |
| ceph-mgr | 11M | Manager 守护进程 |
| radosgw | 16K | RGW 启动器 |
| radosgw-admin | 15M | RGW 管理工具 |
| rados | 772K | RADOS 客户端 |
| rbd | 5.5M | RBD 客户端 |
| rbd-mirror | 8.6M | RBD 镜像守护进程 |
| cephfs-mirror | 7.8M | CephFS 镜像守护进程 |
| ceph-fuse | 7.5M | CephFS FUSE 客户端 |
| ceph-bluestore-tool | 12M | BlueStore 工具 |
| ceph-objectstore-tool | 21M | OSD 离线工具 |

#### Step 3: Strip 并复制库文件

- **核心库**（stage/lib/）：libceph-common、librados、libradosgw、librbd、librgw 等，含 soname 软链接
- **OSD Class 插件**（stage/rados-classes/）：22 个 libcls_*.so 插件
- **Erasure-code 插件**（stage/erasure-code/）：11 个 libec_*.so 插件
- **Python 绑定**（stage/python/）：
  - rados.cpython-36m-x86_64-linux-gnu.so
  - rbd.cpython-36m-x86_64-linux-gnu.so
  - cephfs.cpython-36m-x86_64-linux-gnu.so
  - rgw.cpython-36m-x86_64-linux-gnu.so
  - ceph_argparse.py、ceph_daemon.py（ceph CLI 需要）
  - ceph_volume/（OSD 管理 Python 包）
- **MGR 模块**（stage/mgr/）：alerts、balancer、crash、dashboard、devicehealth、diskprediction_local、iostat、orchestrator、progress、prometheus、rbd_support、status、telemetry、volumes 等 20+ 个模块

#### Step 4: 构建 Docker 镜像

使用 `Dockerfile.production` 构建最终运行时镜像。

### Staging 目录大小

```
142M    stage/bin
69M     stage/lib
8.9M    stage/rados-classes
7.5M    stage/erasure-code
4.0M    stage/python (含 ceph_volume)
4.0K    stage/scripts
18M     stage/mgr
────────────────────
~240M   总计
```

---

## 生产镜像详情（Dockerfile.production）

### 基础镜像

`quay.io/centos/centos:stream8`（与编译环境一致，保证 ABI 兼容）

### 运行时依赖

```
fmt, openssl-libs, libcurl, liboath, openldap, expat, libedit,
gperftools-libs, libibverbs, librdmacm, libnl3, libaio, libblkid,
lz4-libs, snappy, leveldb, ncurses-libs, nss, libcap-ng, libuuid,
systemd-libs, lua-libs, libicu, librabbitmq, librdkafka,
python3, python3-setuptools, python3-prettytable,
smartmontools, e2fsprogs, xfsprogs, parted, gdisk, lvm2,
udev, cryptsetup, kmod
```

### 目录结构

```
/usr/bin/                         # Ceph 二进制
/usr/lib64/                       # Ceph 共享库
/usr/lib64/rados-classes/         # OSD class 插件
/usr/lib64/ceph/erasure-code/     # EC 插件
/usr/lib/python3.6/site-packages/ # Python 绑定 + ceph_volume
/usr/share/ceph/mgr/              # MGR Python 模块
/var/lib/ceph/                    # 数据目录
/var/log/ceph/                    # 日志目录
/etc/ceph/                        # 配置目录
```

### 入口脚本（entrypoint.sh）

支持以第一个参数选择守护进程：

```bash
docker run ceph:16.2.14-fix <daemon> [options]
```

支持的 daemon：`mon`、`osd`、`mds`、`mgr`、`rgw`、`radosgw-admin`、`rados`、`ceph`、`bash`

---

## 使用方式

### 验证镜像

```bash
docker run --rm ceph:16.2.14-fix4 ceph --version
# 输出: ceph version 16.2.14-18-g6491bf5153c (...) pacific (stable)

docker run --rm ceph:16.2.14-fix4 rgw --version
docker run --rm ceph:16.2.14-fix4 mon --version
docker run --rm ceph:16.2.14-fix4 osd --version
docker run --rm ceph:16.2.14-fix4 mgr --version
```

### 运行 MGR

```bash
docker run -d --name ceph-mgr \
    --network host \
    -v /etc/ceph:/etc/ceph:ro \
    -v /var/lib/ceph:/var/lib/ceph \
    ceph:16.2.14-fix4 mgr --name mgr.node1
```

### 运行 RGW

```bash
docker run -d --name ceph-rgw \
    --network host \
    -v /etc/ceph:/etc/ceph:ro \
    -v /var/lib/ceph:/var/lib/ceph \
    ceph:16.2.14-fix4 rgw --name client.rgw.gw0
```

### 运行 MON

```bash
docker run -d --name ceph-mon \
    --network host \
    -v /etc/ceph:/etc/ceph \
    -v /var/lib/ceph:/var/lib/ceph \
    ceph:16.2.14-fix4 mon --id node1
```

### 运行 OSD

```bash
docker run -d --name ceph-osd-0 \
    --network host \
    --privileged \
    -v /etc/ceph:/etc/ceph:ro \
    -v /var/lib/ceph:/var/lib/ceph \
    -v /dev:/dev \
    ceph:16.2.14-fix4 osd --id 0
```

### 执行管理命令

```bash
docker run --rm \
    -v /etc/ceph:/etc/ceph:ro \
    ceph:16.2.14-fix4 ceph -s

docker run --rm \
    -v /etc/ceph:/etc/ceph:ro \
    ceph:16.2.14-fix4 radosgw-admin user list
```

### 进入容器调试

```bash
docker run --rm -it \
    -v /etc/ceph:/etc/ceph:ro \
    ceph:16.2.14-fix4 bash
```

---

## 文件清单

| 文件 | 用途 |
|------|------|
| `build-and-package.sh` | 一键编译 + 打包入口脚本 |
| `build-complete.sh` | 在容器内编译完整 Ceph（含 bundled Boost） |
| `package-production.sh` | strip 二进制 + 构建生产 Docker 镜像 |
| `Dockerfile.production` | 生产运行时镜像定义 |
| `entrypoint.sh` | 容器入口脚本 |
| `install-deps.sh` | 依赖安装脚本（已注释 sepia 内部仓库） |

---

## 构建过程中解决的问题

| 问题 | 原因 | 解决方案 |
|------|------|----------|
| CentOS 8 dnf 报错 | mirrorlist.centos.org DNS 失败（CentOS 8 EOL） | 改用 vault.centos.org |
| install-deps.sh 超时 | sepia 是 Ceph 内部仓库 | 注释掉 sepia 仓库配置 |
| Boost 版本不够 | CentOS 8 只有 Boost 1.66，Ceph 要求 1.72 | 使用 `-DWITH_SYSTEM_BOOST=OFF`（bundled Boost） |
| ninja Boost 未单独执行 | bundled Boost 是 ExternalProject，需先构建 | 在主编译前加 `ninja Boost` |
| cmake configure_file 失败 | 源码目录挂载为只读 | 去掉 docker mount 的 `:ro` 标志 |
| libcurl-minimal 冲突 | CentOS 8 base image 预装了 minimal 版 | dnf 加 `--allowerasing` |
| radosgw 缺 librabbitmq | 运行时依赖未安装 | Dockerfile 加 `librabbitmq` |
| radosgw 缺 librdkafka | 运行时依赖未安装 | Dockerfile 加 `librdkafka` |
| ceph CLI 报 ModuleNotFoundError | 缺 Python rados 绑定和 ceph_argparse.py | 打包 cython modules 和 pybind 脚本 |
| 缺少 ceph-mgr | 旧脚本使用了 `-DWITH_MGR=OFF` | 改为 `-DWITH_MGR=ON` 并重新编译 |
| Docker DNS 解析失败 | 容器无法解析 vault.centos.org | docker build/run 加 `--network=host` |
| 去掉 install-deps.sh 后缺依赖 | Dockerfile 包列表不全，liblttng-ust 等缺失 | builder 镜像中执行 install-deps.sh 并 commit |
| Docker build context 过大（39GB） | COPY 整个源码目录含 build/ | 改用 docker run 挂载 + docker commit |
| mgr/prometheus Python 3.6 不兼容 | `OrderedDict[K, V]` 需 Python 3.9+ | 去掉泛型下标 |

---

## 重要设计决策

1. **使用 bundled Boost 而非修改源代码适配系统 Boost**
   - 官方构建方式，代码改动最小化
   - 不引入兼容性 hack，只包含实际的 bug 修复

2. **编译完整 Ceph 而非仅 RGW**
   - 生产环境需要完整组件（mon + osd + mds + rgw）
   - 统一版本避免混用不同编译的二进制

3. **运行时镜像与编译环境使用相同 OS（CentOS Stream 8）**
   - 保证 glibc、OpenSSL 等系统库 ABI 兼容
   - 避免运行时 .so 版本不匹配

4. **strip debug info 但保留符号**
   - `--strip-debug` 而非 `--strip-all`
   - 产物从 20GB 压缩到 230MB，仍可用 perf/gdb 基本调试

5. **Builder 镜像复用 + install-deps.sh 固化**
   - 首次构建通过 `docker run` + `docker commit` 将 install-deps.sh 的安装结果固化到镜像
   - 后续增量编译直接复用镜像，跳过依赖安装步骤，节省 10-15 分钟
   - 使用挂载而非 COPY 避免 39GB build context 传输

---

## 验证修复

构建完成后，使用 `rgw-multidel-repro/` 测试程序验证 UAF 修复：

```bash
cd rgw-multidel-repro
go run main.go \
    -endpoint http://<rgw-host>:8000 \
    -ak ACCESS_KEY \
    -sk SECRET_KEY \
    -bucket test-multidel \
    -distinct 8 \
    -dup 64 \
    -workers 32 \
    -rounds 1000
```

**预期结果**：
- 原版 16.2.14：几分钟内 RGW crash
- 修复版 16.2.14-fix：稳定运行不 crash
