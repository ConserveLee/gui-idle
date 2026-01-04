package constants

import "time"

// Global Expedition Configuration
const (
	// Scan Intervals
	EntryScanIntervalHighSpeed = 150 * time.Millisecond // Scanning interval when idle in Entry state
	EntryRetryInterval         = 0                      // Interval when retrying immediately (loop fast)
	InGameScanInterval         = 30 * time.Second       // Low frequency scan while in game
	SearchScanInterval         = 2 * time.Second        // Scan interval for search steps
	SearchRetryInterval        = 500 * time.Millisecond // Fast retry interval for search states

	// Retry Limits
	SearchMaxRetries = 3 // Max retries before falling back to AutoDetect

	// Interaction Delays
	WaitAfterClickQuick  = 100 * time.Millisecond // Quick wait after clicking Entry
	WaitAfterClickNormal = 1 * time.Second        // Standard wait after clicking Search/Exit buttons

	// Verification
	EntryVerifyTimeout = 5 * time.Second
	VerifyPreWait      = 200 * time.Millisecond // Wait before starting verification (screen transition)
	VerifyRetryWait    = 200 * time.Millisecond // Wait between verification attempts
	VerifyLoadingWait  = 300 * time.Millisecond // Wait when screen state is loading/unrecognized

	// Entity Tracker
	EntityTTL = 2 * time.Second // Time before a tracked entity is removed if not seen

	// Image Matching
	DefaultTolerance = 60    // Color tolerance for pixel comparison
	MaxFailRate      = 0.03  // Allow up to 3% of pixels to fail matching
	MaxPixelDiff     = 150.0 // Maximum allowed color diff for any pixel (reject if exceeded)

	// Debugging
	DebugDump = true
)
