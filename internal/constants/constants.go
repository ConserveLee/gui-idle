package constants

import "time"

// Global Expedition Configuration
const (
	// Scan Intervals
	EntryScanIntervalHighSpeed = 400 * time.Millisecond // Scanning interval when idle in Entry state
	EntryRetryInterval         = 0                      // Interval when retrying immediately (loop fast)
	InGameScanInterval         = 30 * time.Second       // Low frequency scan while in game
	SearchScanInterval         = 2 * time.Second        // Scan interval for search steps

	// Interaction Delays
	WaitAfterClickQuick  = 100 * time.Millisecond // Quick wait after clicking Entry
	WaitAfterClickNormal = 1 * time.Second        // Standard wait after clicking Search/Exit buttons

	// Verification
	EntryVerifyTimeout = 5 * time.Second

	// Entity Tracker
	EntityTTL = 2 * time.Second // Time before a tracked entity is removed if not seen

	// Image Matching
	DefaultTolerance = 60

	// Debugging
	DebugDump = true
)
