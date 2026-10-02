// One verify run's throwaway PLC and PDS; bin/verify owns the ports, the data dir and the process lifetime.
import fs from 'node:fs/promises'
import { TestNetworkNoAppView } from '@atproto/dev-env'

const dir = process.env.PDS_DATA_DIR
// Without an explicit data directory dev-env leaves two temp dirs behind per instance.
await fs.mkdir(`${dir}/blobs`, { recursive: true })
const net = await TestNetworkNoAppView.create({
  pds: { port: Number(process.env.PDS_PORT), dataDirectory: dir, blobstoreDiskLocation: `${dir}/blobs` },
  plc: { port: Number(process.env.PLC_PORT) },
})
console.log(JSON.stringify({ pds: net.pds.url, plc: net.plc.url }))

const stop = async () => {
  await net.close()
  process.exit(0)
}
process.on('SIGTERM', stop)
process.on('SIGINT', stop)
