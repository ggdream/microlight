wa-sqlite v1.2.0, upstream Git tag commit:
4631745762f5e24d226254158e5ceada2d5fbe1e

Source: https://github.com/rhashimoto/wa-sqlite/tree/v1.2.0
License: MIT (see LICENSE).

This directory contains unmodified upstream distribution files and only the
JavaScript modules required by IDBBatchAtomicVFS. The Asyncify build supports
asynchronous IndexedDB without JSPI or SharedArrayBuffer. Runtime assets are
served from the same origin, not a CDN. No npm or C toolchain is needed to build
the application. SHA256SUMS pins every shipped upstream file; scripts/web-assets.sh
checks it when staging the Web build. To update, review upstream release notes,
copy matching source AND distribution files, regenerate checksums, and run the
browser storage acceptance suite before changing this version.
