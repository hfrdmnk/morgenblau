// One verify run's throwaway PLC and PDS; bin/verify owns the ports, the data dir, the secrets and the process lifetime.
import fs from 'node:fs/promises'
import http from 'node:http'
import { TestNetworkNoAppView } from '@atproto/dev-env'

// The PDS and PLC call listen(port) with no host, which binds every interface; the run's account must stay unreachable from the network.
const listen = http.Server.prototype.listen
http.Server.prototype.listen = function (port, ...rest) {
  if (typeof port === 'number' && typeof rest[0] !== 'string') return listen.call(this, port, '127.0.0.1', ...rest)
  return listen.call(this, port, ...rest)
}

const dir = process.env.PDS_DATA_DIR
// Without an explicit data directory dev-env leaves two temp dirs behind per instance.
await fs.mkdir(`${dir}/blobs`, { recursive: true })
const net = await TestNetworkNoAppView.create({
  pds: {
    port: Number(process.env.PDS_PORT),
    dataDirectory: dir,
    blobstoreDiskLocation: `${dir}/blobs`,
    adminPassword: process.env.PDS_ADMIN_PASSWORD,
    jwtSecret: process.env.PDS_JWT_SECRET,
  },
  plc: { port: Number(process.env.PLC_PORT) },
})
console.log(JSON.stringify({ pds: net.pds.url, plc: net.plc.url }))

const stop = async () => {
  await net.close()
  process.exit(0)
}
process.on('SIGTERM', stop)
process.on('SIGINT', stop)
