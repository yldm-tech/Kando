//go:build !race

package sourcetest

import "time"

// callWait is how long Call waits for an answer: the 15 s Kando gives a source's list, stat and open.
const callWait = 15 * time.Second
