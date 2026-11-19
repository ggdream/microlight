package desktop

import "runtime"

// Mobile describes OS capabilities. Layout decisions use the viewport instead.
const Mobile = runtime.GOOS == "ios" || runtime.GOOS == "android"
