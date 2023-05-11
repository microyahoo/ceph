// gcc -g rados_write.c -lrados -L/root/go/src/ceph/build/lib -o rados_write -Wl,-rpath,/root/go/src/ceph/build/
// https://www.dovefi.com/post/%E6%B7%B1%E5%85%A5%E7%90%86%E8%A7%A3crush2%E6%89%8B%E5%8A%A8%E7%BC%96%E8%AF%91ceph%E9%9B%86%E7%BE%A4%E5%B9%B6%E4%BD%BF%E7%94%A8librados%E8%AF%BB%E5%86%99%E6%96%87%E4%BB%B6/
// https://docs.ceph.com/en/quincy/rados/api/librados-intro/
//
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <rados/librados.h>

int
main (int argc, const char* argv[])
{
        rados_t cluster;
        char cluster_name[] = "ceph";           // 集群名字，默认为ceph
        char user_name[] = "client.admin";      // 用户名称
        char conf_file[] = "/root/go/src/ceph/build/ceph.conf"; // 配置文件路径
        char poolname[] = "default.rgw.meta";                   // 存储池名字
        char objname[] = "test-write";                          // 对象名字
        char obj_content[] = "Hello World, ceph!";              // 对象内容
        uint64_t flags = 0;

        /* Initialize the cluster handle with the "ceph" cluster name and the "client.admin" user */
        int err;
        err = rados_create2(&cluster, cluster_name, user_name, flags);
        if (err < 0) {
                fprintf(stderr, "%s: Couldn't create the cluster handle! %s\n", argv[0], strerror(-err));
                exit(EXIT_FAILURE);
        } else {
                printf("\nCreated a cluster handle.\n");
        }


        /* Read a Ceph configuration file to configure the cluster handle. */
        //err = rados_conf_read_file(cluster, "/etc/ceph/ceph.conf");
        err = rados_conf_read_file(cluster, conf_file);
        if (err < 0) {
                fprintf(stderr, "%s: cannot read config file: %s\n", argv[0], strerror(-err));
                exit(EXIT_FAILURE);
        } else {
                printf("\nRead the config file.\n");
        }

        /* Read command line arguments */
        err = rados_conf_parse_argv(cluster, argc, argv);
        if (err < 0) {
                fprintf(stderr, "%s: cannot parse command line arguments: %s\n", argv[0], strerror(-err));
                exit(EXIT_FAILURE);
        } else {
                printf("\nRead the command line arguments.\n");
        }

        /* Connect to the cluster */
        err = rados_connect(cluster);
        if (err < 0) {
                fprintf(stderr, "%s: cannot connect to cluster: %s\n", argv[0], strerror(-err));
                exit(EXIT_FAILURE);
        } else {
                printf("\nConnected to the cluster.\n");
        }

        /*
         * Continued from previous C example, where cluster handle and
         * connection are established. First declare an I/O Context.
         */
        rados_ioctx_t io;
        err = rados_ioctx_create(cluster, poolname, &io);
        if (err < 0) {
                fprintf(stderr, "%s: cannot open rados pool %s: %s\n", argv[0], poolname, strerror(-err));
                rados_shutdown(cluster);
                exit(EXIT_FAILURE);
        } else {
                printf("\nCreated I/O context.\n");
        }

        /* Write data to the cluster synchronously. */
        err = rados_write(io, objname, obj_content, 16, 0);
        if (err < 0) {
                fprintf(stderr, "%s: Cannot write object \"test-write\" to pool %s: %s\n", argv[0], poolname, strerror(-err));
                rados_ioctx_destroy(io);
                rados_shutdown(cluster);
                exit(1);
        } else {
                printf("\nWrote \"Hello World\" to object \"test-write\".\n");
        }

        char xattr[] = "en_US";
        err = rados_setxattr(io, "test-write", "lang", xattr, 5);
        if (err < 0) {
                fprintf(stderr, "%s: Cannot write xattr to pool %s: %s\n", argv[0], poolname, strerror(-err));
                rados_ioctx_destroy(io);
                rados_shutdown(cluster);
                exit(1);
        } else {
                printf("\nWrote \"en_US\" to xattr \"lang\" for object \"test-write\".\n");
        }

        /*
         * Read data from the cluster asynchronously.
         * First, set up asynchronous I/O completion.
         */
        rados_completion_t comp;
        err = rados_aio_create_completion(NULL, NULL, NULL, &comp);
        if (err < 0) {
                fprintf(stderr, "%s: Could not create aio completion: %s\n", argv[0], strerror(-err));
                rados_ioctx_destroy(io);
                rados_shutdown(cluster);
                exit(1);
        } else {
                printf("\nCreated AIO completion.\n");
        }

        /* Next, read data using rados_aio_read. */
        char read_res[100];
        err = rados_aio_read(io, "hw", comp, read_res, 12, 0);
        if (err < 0) {
                fprintf(stderr, "%s: Cannot read object. %s %s\n", argv[0], poolname, strerror(-err));
                rados_ioctx_destroy(io);
                rados_shutdown(cluster);
                exit(1);
        } else {
                printf("\nRead object \"hw\". The contents are:\n %s \n", read_res);
        }

        /* Wait for the operation to complete */
        rados_aio_wait_for_complete(comp);

        /* Release the asynchronous I/O complete handle to avoid memory leaks. */
        rados_aio_release(comp);


        char xattr_res[100];
        err = rados_getxattr(io, objname, "lang", xattr_res, 5);
        if (err < 0) {
                fprintf(stderr, "%s: Cannot read xattr. %s %s\n", argv[0], poolname, strerror(-err));
                rados_ioctx_destroy(io);
                rados_shutdown(cluster);
                exit(1);
        } else {
                printf("\nRead xattr \"lang\" for object \"test-write\". The contents are:\n %s \n", xattr_res);
        }

        /* err = rados_rmxattr(io, objname, "lang"); */
        /* if (err < 0) { */
        /*         fprintf(stderr, "%s: Cannot remove xattr. %s %s\n", argv[0], poolname, strerror(-err)); */
        /*         rados_ioctx_destroy(io); */
        /*         rados_shutdown(cluster); */
        /*         exit(1); */
        /* } else { */
        /*         printf("\nRemoved xattr \"lang\" for object \"test-write\".\n"); */
        /* } */

        /* err = rados_remove(io, objname); */
        /* if (err < 0) { */
        /*         fprintf(stderr, "%s: Cannot remove object. %s %s\n", argv[0], poolname, strerror(-err)); */
        /*         rados_ioctx_destroy(io); */
        /*         rados_shutdown(cluster); */
        /*         exit(1); */
        /* } else { */
        /*         printf("\nRemoved object \"test-write\".\n"); */
        /* } */

        rados_ioctx_destroy(io);
        rados_shutdown(cluster);

        return 0;
}
