package film

// The tests link NSGo's backends, so that nsgo test -target runs them on the platform, where nsgotest captures what it shows (checkOutlinePixels); a package whose tests link no backend runs them headless even there. go test runs them headless as before.
import _ "github.com/nsgo-dev/nsgo"
