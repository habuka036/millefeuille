package millefeuille

import "time"

// ClockFunc is an injectable time source. The production default is time.Now.
// Tests may supply a fixed function to produce deterministic timestamps.
type ClockFunc func() time.Time
