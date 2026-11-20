# Browser storage

Microlight keeps its existing `database/sql` store, SQL schema and migrations.
Native desktop and mobile builds still use `modernc.org/sqlite`, with no cgo
requirement introduced by Web support. Browser builds select
`pkg/sqliteweb` through `internal/storage/database_js.go`; native imports and
file migration are isolated behind `!js` constraints. Neither Web driver nor
its JavaScript assets are dependencies of native application binaries.

## Implementation

The Web driver bridges Go to **wa-sqlite v1.2.0**, running SQLite in a dedicated
module Worker. The unmodified upstream **IDBBatchAtomicVFS** stores database
pages and metadata in IndexedDB. It is not an in-memory database periodically
exported as a whole-file snapshot. SQL transactions use SQLite's normal
transaction machinery and the VFS's persistent transaction/version handling.
`synchronous=FULL` is applied on connection startup. A successful SQL commit
waits for the VFS's durability work; no timer or unload handler is required to
save committed changes.

The Worker serializes all calls into its Asyncify SQLite instance. Each
`sql.DB` uses one connection, matching the native store. A database can be
opened once per tab; callers share that `sql.DB`. Separate tabs use separate
Workers and upstream Web Locks to coordinate access. Writes serialize; this
does not limit the number of concurrently running agent conversations.

The bridge preserves signed 64-bit integers using decimal strings at the
message boundary, distinguishes INTEGER from REAL, and transfers BLOBs as
binary typed arrays. NULL, empty TEXT and empty BLOB remain distinct. Rows are
stepped on demand rather than materializing an entire query result in Go.
Prepared statements and cursors are finalized or reset when closed. The bridge
explicitly awaits asynchronous statement finalization, including startup PRAGMAs
and internal queries; upstream iterator auto-cleanup does not await Asyncify
finalization. Repeated read/close cycles are covered by the browser suite.

MyGo Web invokes view/input callbacks synchronously from the browser. Database
work must not wait for a Worker inside those callbacks. The Web app therefore
serializes persistence actions on a separate Go coroutine; nested actions run
in order on that coroutine. Native builds use a no-op dispatcher and retain
their existing synchronous store calls. The acceptance suite includes actual
production-UI text input and a reload to cover this integration boundary.

Cancellation is checked before dispatch. An already submitted operation is
drained before returning its connection to the pool, so cancellation cannot
cause a late write to overlap a subsequent operation. The bridge currently
does not interrupt a long-running SQLite instruction in the Worker.

Upstream files, license, pinned commit and checksums are in
[`pkg/sqliteweb/web/vendor`](../pkg/sqliteweb/web/vendor). There is no CDN,
runtime npm dependency, JS eval, or custom SQLite VFS implementation.
The Asyncify distribution avoids requiring JSPI or SharedArrayBuffer.

Sources reviewed:

- [wa-sqlite README](https://github.com/rhashimoto/wa-sqlite/tree/v1.2.0)
- [VFS capabilities and limitations](https://github.com/rhashimoto/wa-sqlite/blob/v1.2.0/src/examples/README.md)
- [IndexedDB VFS implementation](https://github.com/rhashimoto/wa-sqlite/blob/v1.2.0/src/examples/IDBBatchAtomicVFS.js)
- [Web Locks implementation](https://github.com/rhashimoto/wa-sqlite/blob/v1.2.0/src/WebLocksMixin.js)

## Build and deployment

```sh
CGO_ENABLED=0 bash scripts/build.sh -platform web
```

`scripts/web-assets.sh` verifies upstream file checksums and stages the Worker,
bridge, SQLite Wasm and required JavaScript beside MyGo's output in
`dist/web/sqlite/`. It also handles `-o` and `-platform=web`. Use the repository
build script: calling MyGo directly does not stage these application assets.
Node/npm is not needed for the application build.

Serve the entire directory over HTTPS (localhost HTTP works for development),
with `.js`/`.mjs` as JavaScript and `.wasm` as `application/wasm`. The browser
needs WebAssembly, module Workers, IndexedDB and Web Locks. COOP/COEP response
headers are not required for this backend. A CSP must allow the same-origin
scripts/Workers and Wasm compilation (`wasm-unsafe-eval` where required by the
browser). Missing assets or storage failures report an error; the app does not
silently substitute a transient database and overwrite existing data.

Data is local to the site's origin and browser profile. Browser storage quotas,
private-browsing restrictions and clearing site data still apply. Database
pages are not encrypted. Changing the host, protocol or port changes the data
origin. Config import/export transfers settings and keys, not conversations.

## Platform capabilities

Web startup opens IndexedDB directly and skips OS data-directory migration,
file logging, tray, global shortcut registration, updater and process runners.
Local filesystem/workspace tools, on-disk skills, MCP stdio and Chromium
automation are not registered in Web agents. Native-only settings are hidden.
HTTP MCP, configured model/search/OCR APIs and remote images depend on the
remote endpoint permitting browser CORS; this app cannot bypass that policy.
Preview requests reject obvious local hostnames and non-public IP literals;
domain resolution and network access are governed by the browser, rather than
the native preview client's socket-level DNS validation.

Browser HTTP uses Fetch and the browser's network/proxy configuration. Imported
native proxy settings are retained for config portability but cannot install
an application-level proxy in the browser. The Windows proxy implementation
and Android/iOS system proxy modules are unchanged.

Attachment selection uses browser file input; saved bytes remain in SQLite
BLOB columns. Browsers cannot expose an actual local absolute path, so Web
attachment metadata has no usable OS path. File and config downloads use Blob
downloads, and attachment previews use sandboxed browser content. Native file
dialogs, Quick Look and mobile sharing continue to use their existing APIs.

## Acceptance checks

The optional test runner requires Node and Playwright only for testing:

```sh
npm ci --prefix scripts/sqlitewebcheck
cd scripts/sqlitewebcheck
npx playwright install chromium
npm test
```

Alternatively set `PLAYWRIGHT_CHROMIUM_EXECUTABLE` to an installed Chromium
browser. The runner builds Go/Wasm with `CGO_ENABLED=0`, creates a temporary
localhost server and a new browser profile, and cleans them up afterward.
It never reads native application settings or keys.

Browser acceptance (including storage, UI input and notification checks) runs
locally when needed, not in CI. CI runs the Go HTTP-server tests and builds the
Web release packages; it does not install Node, Playwright or a browser. System
banners and clicking them require local visual verification with the browser's
OS notification permission enabled.

Coverage:

- A 12 MiB binary value, int64 beyond JavaScript's exact number range, REAL,
  embedded NUL text, NULL and empty BLOB/TEXT.
- SQL rollback and a page reload with exact byte/hash comparisons.
- Two tabs writing the same database: 80 atomic increments without lost writes.
- IndexedDB instrumentation verifies 40 later small updates write less than
  1 MiB of block data, rather than 40 full database snapshots.
- Closing a tab with a 16 MiB uncommitted update, reopening in another tab,
  checking prior committed data and `PRAGMA integrity_check`.
- The application's real schema migration, history/message read/write, 4 MiB
  attachment, draft and configuration marker across a page reload.
- Browser file selection and original-byte download, plus actual production
  MyGo UI input: settings and composer drafts remain responsive and survive
  a reload. This catches synchronous-callback deadlocks a driver-only test misses.

The initial device-independent browser acceptance was run on macOS Chromium.
Firefox and Safari are not claimed as tested by that result. The upstream
VFS's browser compatibility does not substitute for application acceptance.

## Release packages and standalone server

`bash scripts/package-web.sh` produces exactly two Web release archives:

- `microlight-vX.Y.Z-web-static.tar.gz`: the complete static site, with
  `index.html` at the archive root. Deploy at the origin root. Configure your
  host to fall back to `/index.html` for History page routes, while returning
  404 for missing assets.
- `microlight-vX.Y.Z-web-servers.tar.gz`: standalone, no-cgo binaries for
  Windows amd64, Linux amd64/arm64 and macOS amd64/arm64, plus checksums and
  usage instructions. Each binary embeds the identical site using `go:embed`.

Run the matching binary with `-addr :8080` (the default) and visit
`http://localhost:8080/`. It listens on all interfaces, allowing other devices
on your network to connect. Use an HTTPS reverse proxy for remote deployment:
Web Locks and other browser APIs require a secure context outside localhost.
This is a static application server, not a shared configuration/database API;
each browser stores its own local data and model credentials.

The server supports GET/HEAD, directory index files, MIME types and extensionless
HTML History fallback. Missing assets and directories without an index return
404; directory listings are disabled. The generated root `<base href="/">`
keeps Worker, Wasm and script URLs correct after deep-link reloads. Revalidation
prevents a stale cached index from referring to deleted Wasm assets.

Generated embed assets are staged under `cmd/webserver/site`, excluded from Git
and removed when packaging finishes. A regular `go run ./cmd/webserver` without
staging exits with build instructions instead of serving an empty application.
`MICROLIGHT_WEB_SITE` may point to an already tested Web build to reuse it;
`MICROLIGHT_BUILD_VERSION` controls archive names.

CI runs the Go HTTP-server tests and production Web build on every push and PR,
then builds all five server targets and uploads the two archives as one Actions
artifact. Browser acceptance runs locally when needed. Tag releases wait for this job and publish both archives
alongside the desktop and mobile assets. Mobile CI still uses compilation only,
without simulators, installation or screenshots.

## Browser notifications

Web uses the existing notification scope and test button in Settings. The
Go/Wasm-only adapter in `pkg/webnotify` uses the typed
[gowebapi notification binding v0.2.0](https://github.com/gowebapi/webapi/tree/v0.2.0/notification).
Native notifications still use MyGo. Requesting permission starts synchronously
from the user's notification selection or test button; asynchronous waiting does
not block MyGo's JavaScript callbacks. Background replies never request permission.
Granted/denied state is read from the browser, independently of imported settings.

The browser requires HTTPS or a trusted localhost context. The root
`notification-worker.js` is included in both Web packages. Service Worker
notifications support browsers that prohibit the page Notification constructor;
a page notification is used when Service Workers are unavailable. Notifications
carry their originating client ID, so clicking focuses the correct tab and routes
the existing conversation action back to Go. The worker does not proxy requests,
cache application files or subscribe to Web Push.

Replies, errors, clarification and approval notifications follow the same filters
as native builds. Foreground replies remain silent; the test button can send while
the page is visible. Permission denials direct the user to browser site settings.
Delivery depends on browser/OS notification preferences; an accepted API call does
not guarantee that the operating system displays a banner.

This supports notifications while the page is running, not remote delivery after
closing it. Some mobile browsers require an installed home-screen web app for
notifications; this change does not add a Web Push server or PWA installation flow.
See [Notifications API](https://developer.mozilla.org/en-US/docs/Web/API/Notifications_API).
The browser acceptance suite checks actual permission grant/denial, worker delivery,
notification contents, originating tab identity and closing, plus the production
settings test button and a background reply through a local deterministic model
fixture. It does not claim to verify an OS banner or a physical OS-notification click.
