#include "Protocol.h"

#include "AsyncConnection.h"
#include "AsyncMessenger.h"

Protocol::Protocol(int type, AsyncConnection *connection)
  : proto_type(type),
    connection(connection), // 设置 protocol 对应的 async connection
    messenger(connection->async_msgr), // 设置 protocol 对应的 async messenger
    cct(connection->async_msgr->cct) {
  auth_meta.reset(new AuthConnectionMeta());
}

Protocol::~Protocol() {}
