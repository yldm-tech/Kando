//go:build race

package sourcetest

import "time"

// callWait is how long Call waits for an answer under the race detector, which makes the script runner several times slower: a list of more films than a page (TestPluginJellyfinPages' 1203) took over the 15 s Kando gives it, which says nothing of the plugin.
const callWait = 60 * time.Second
