package adapter

import "time"

// ist is the zone the fixtures were recorded in (testdata/stats.json is +05:30).
// Every test clock is built in it, and building it makes it the local zone, so
// the tests read days and deadlines the same on CI (UTC) as on his laptop.
var ist = func() *time.Location {
	l := time.FixedZone("IST", 5*3600+30*60)
	time.Local = l
	return l
}()
