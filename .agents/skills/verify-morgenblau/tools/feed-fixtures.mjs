// One verify run's fixture feed server and its decoy; bin/verify owns the ports and the process lifetime.
// The server's FEED_FIXTURE_ORIGIN admits only FIXTURE_PORT, so anything served from DECOY_PORT must stay unreachable from the app.
import fs from 'node:fs'
import http from 'node:http'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const dir = path.join(path.dirname(fileURLToPath(import.meta.url)), '../fixtures/feeds')
const fixturePort = Number(process.env.FIXTURE_PORT)
const decoyPort = Number(process.env.DECOY_PORT)
const origin = `http://127.0.0.1:${fixturePort}`
const decoyOrigin = `http://127.0.0.1:${decoyPort}`
// gofeed reads up to the fetcher's 5 MiB cap; one byte more makes the feed too large.
const HUGE_BYTES = 5 * 1024 * 1024 + 1024
let toggleUp = true
let undatedLater = false

// Entries dated a minute before the request land in today's digest whatever time the run starts, except in the first minute after midnight.
function render(name) {
  const now = new Date(Date.now() - 60_000)
  return fs.readFileSync(path.join(dir, name), 'utf8')
    .replaceAll('{{ORIGIN}}', origin)
    .replaceAll('{{NOW_RFC822}}', now.toUTCString().replace('GMT', '+0000'))
    .replaceAll('{{NOW_RFC3339}}', now.toISOString())
    .replaceAll('{{LATER}}', undatedLater ? '<item><title>Fixture undated entry found later</title><link>' + origin + '/posts/undated-later</link><guid isPermaLink="false">example-undated-later</guid></item>\n' : '')
}

function send(res, status, type, body) {
  res.writeHead(status, { 'content-type': type, 'cache-control': 'no-store' })
  res.end(body)
}

function huge(res) {
  res.writeHead(200, { 'content-type': 'application/rss+xml; charset=utf-8' })
  res.write('<?xml version="1.0" encoding="UTF-8"?><rss version="2.0"><channel><title>Example Huge Feed</title>')
  const item = `<item><title>Padding entry</title><description>${'x'.repeat(4000)}</description></item>`
  for (let written = 0; written < HUGE_BYTES; written += item.length) res.write(item)
  res.end('</channel></rss>')
}

const feeds = {
  '/feeds/valid.xml': () => ['application/rss+xml; charset=utf-8', render('valid.xml')],
  '/feeds/atom.xml': () => ['application/atom+xml; charset=utf-8', render('atom.xml')],
  '/feeds/stale.xml': () => ['application/rss+xml; charset=utf-8', render('stale.xml')],
  '/feeds/broken.xml': () => ['application/rss+xml; charset=utf-8', render('broken.xml')],
  '/feeds/undated.xml': () => ['application/rss+xml; charset=utf-8', render('undated.xml')],
  '/feeds/latin1.xml': () => ['application/rss+xml; charset=ISO-8859-1', Buffer.from(render('latin1.xml'), 'latin1')],
  // The site's SocialSpec profile (socialspec.org), read by the add-source sheet.
  '/.well-known/about.md': () => ['text/markdown; charset=utf-8', render('well-known/about.md')],
  '/.well-known/contact.vcf': () => ['text/vcard; charset=utf-8', render('well-known/contact.vcf')],
  '/.well-known/following.opml': () => ['text/x-opml+xml; charset=utf-8', render('well-known/following.opml')],
}

const fixtures = http.createServer((req, res) => {
  const { pathname } = new URL(req.url, origin)
  if (req.method === 'POST' && /^\/control\/toggle\/(up|down)$/.test(pathname)) {
    toggleUp = pathname.endsWith('/up')
    return send(res, 200, 'application/json', JSON.stringify({ toggle: toggleUp ? 'up' : 'down' }))
  }
  if (req.method === 'POST' && pathname === '/control/undated/later') {
    undatedLater = true
    return send(res, 200, 'application/json', JSON.stringify({ undated: 'later' }))
  }
  if (req.method !== 'GET' && req.method !== 'HEAD') return send(res, 405, 'text/plain', 'method not allowed\n')
  if (feeds[pathname]) return send(res, 200, ...feeds[pathname]())
  switch (pathname) {
    case '/':
      return send(res, 200, 'text/html; charset=utf-8', render('index.html'))
    case '/feeds/huge.xml':
      return huge(res)
    case '/feeds/unavailable.xml':
      return send(res, 503, 'text/plain', 'fixture unavailable\n')
    case '/feeds/toggle.xml':
      return toggleUp ? send(res, 200, 'application/rss+xml; charset=utf-8', render('toggle.xml')) : send(res, 503, 'text/plain', 'toggle is down\n')
    case '/feeds/redirect-decoy.xml':
      res.writeHead(302, { location: `${decoyOrigin}/feed.xml` })
      return res.end()
  }
  if (pathname.startsWith('/posts/')) return send(res, 200, 'text/html; charset=utf-8', render('post.html'))
  send(res, 404, 'text/plain', 'no such fixture\n')
})

const decoy = http.createServer((req, res) => {
  send(res, 200, 'application/rss+xml; charset=utf-8',
    `<?xml version="1.0" encoding="UTF-8"?><rss version="2.0"><channel><title>Decoy feed that must stay unreachable</title><link>${decoyOrigin}/</link><item><title>Decoy entry</title><link>${decoyOrigin}/posts/decoy</link><guid>example-decoy</guid></item></channel></rss>`)
})

const listen = (server, port) => new Promise((resolve, reject) => {
  server.once('error', reject)
  server.listen(port, '127.0.0.1', () => resolve(server.address()))
})
// The bound addresses come from the sockets, so a caller can check the listeners stay on loopback.
const [fixturesAddress, decoyAddress] = await Promise.all([listen(fixtures, fixturePort), listen(decoy, decoyPort)])
console.log(JSON.stringify({ fixtures: fixturesAddress, decoy: decoyAddress }))

const stop = () => {
  fixtures.close()
  decoy.close()
  process.exit(0)
}
process.on('SIGTERM', stop)
process.on('SIGINT', stop)
