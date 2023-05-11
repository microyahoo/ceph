import rados

try:
        cluster = rados.Rados(conffile='/root/go/src/ceph/build/ceph.conf')
except TypeError as e:
        print('Argument validation error: {}'.format(e))
        raise e

print("Created cluster handle.")

try:
        cluster.connect()
except Exception as e:
        print("connection error: {}".format(e))
        raise e
finally:
        print("Connected to the cluster.")

print("\n\nI/O Context and Object Operations")
print("=================================")

print("\nCreating a context for the 'default.rgw.meta' pool")
if not cluster.pool_exists('default.rgw.meta'):
        raise RuntimeError('No default.rgw.meta pool exists')
ioctx = cluster.open_ioctx('default.rgw.meta')

print("\nWriting object 'hw' with contents 'Hello World!' to pool 'default.rgw.meta'.")
ioctx.write("hw", b"Hello World!")
print("Writing XATTR 'lang' with value 'en_US' to object 'hw'")
ioctx.set_xattr("hw", "lang", b"en_US")


print("\nWriting object 'bm' with contents 'Bonjour tout le monde!' to pool 'default.rgw.meta'.")
ioctx.write("bm", b"Bonjour tout le monde!")
print("Writing XATTR 'lang' with value 'fr_FR' to object 'bm'")
ioctx.set_xattr("bm", "lang", b"fr_FR")

print("\nContents of object 'hw'\n------------------------")
print(ioctx.read("hw"))

print("\n\nGetting XATTR 'lang' from object 'hw'")
print(ioctx.get_xattr("hw", "lang"))

print("\nContents of object 'bm'\n------------------------")
print(ioctx.read("bm"))

print("\n\nGetting XATTR 'lang' from object 'bm'")
print(ioctx.get_xattr("bm", "lang"))


# print("\nRemoving object 'hw'")
# ioctx.remove_object("hw")

# print("Removing object 'bm'")
# ioctx.remove_object("bm")

print("\nClosing the connection.")
ioctx.close()

print("Shutting down the handle.")
cluster.shutdown()
