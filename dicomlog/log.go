package dicomlog

import (
	"github.com/sirupsen/logrus"
	"sync/atomic"
)

// level sets log verbosity. The larger the value, the more verbose.  Setting it
// to -1 disables logging completely.
var level = int32(0)

// SetLevel sets log verbosity. The larger the value, the more verbose. Setting
// it to -1 disables logging completely. Thread safe.
func SetLevel(l int) {
	atomic.StoreInt32(&level, int32(l))
}

// Level returns the current log level. The larger the value, the more verbose.
// Thread safe.
func Level() int {
	return int(atomic.LoadInt32(&level))
}

// Vprintf is shorthand for "if level > Level { log.Printf(...) }".
func Vprintf(l int, format string, args ...interface{}) {
	if Level() >= l {
		logrus.Printf(format, args...)
	}
}
