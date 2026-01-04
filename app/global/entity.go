package global

import (
	"image"
	"regexp"
	"sort"
	"strconv"
	"sync"
	"time"
)

// DetectedEntity represents an entry button detected on screen
type DetectedEntity struct {
	TemplateName string      // Template filename (e.g., "20.png")
	Priority     int         // Number extracted from filename (e.g., 20)
	Position     image.Point // Top-left position on screen
	TemplateSize image.Point // Template dimensions (for center calculation)
}

// TrackedEntity wraps DetectedEntity with tracking metadata
type TrackedEntity struct {
	Entity     DetectedEntity
	ClickCount int       // Number of times this entity has been clicked
	LastSeen   time.Time // Last time this entity was detected
	FirstSeen  time.Time // First time this entity was detected
}

// EntityTracker manages entity lifecycle: tracking, counting, and blacklisting
type EntityTracker struct {
	mu             sync.Mutex
	entities       map[string]*TrackedEntity // Active tracked entities
	blacklist      map[string]time.Time      // Blacklisted entity keys with timestamp
	maxClicks      int                       // Max clicks before blacklisting (default: 7)
	positionThresh int                       // Position matching threshold in pixels (default: 20)
	ttl            time.Duration             // Time-to-live for entities (default: 2s)

	// ROI (Region of Interest) for fast detection
	lastHighPriEntity *DetectedEntity // Last detected high priority entity
	roiMargin         int             // Margin around last position for ROI (default: 100px)

	// Debug callback
	debugFunc func(string, ...interface{})
}

// NewEntityTracker creates a new tracker with default settings
func NewEntityTracker() *EntityTracker {
	return &EntityTracker{
		entities:       make(map[string]*TrackedEntity),
		blacklist:      make(map[string]time.Time),
		maxClicks:      7,
		positionThresh: 20,
		ttl:            2 * time.Second,
		roiMargin:      100, // 100px margin around last high priority entity
		debugFunc:      func(string, ...interface{}) {}, // No-op by default
	}
}

// SetDebugFunc sets the debug logging function
func (t *EntityTracker) SetDebugFunc(f func(string, ...interface{})) {
	t.debugFunc = f
}

// entityKey generates a unique key for an entity based on priority and position
func (t *EntityTracker) entityKey(e DetectedEntity) string {
	// Quantize position to allow small movement tolerance
	qx := (e.Position.X / t.positionThresh) * t.positionThresh
	qy := (e.Position.Y / t.positionThresh) * t.positionThresh
	return strconv.Itoa(e.Priority) + "_" + strconv.Itoa(qx) + "_" + strconv.Itoa(qy)
}

// Update processes newly detected entities:
// - Updates LastSeen for existing entities
// - Adds new entities
// - Removes expired entities (not seen for TTL duration)
// - Handles Y-axis movement (entities moving up in the list)
func (t *EntityTracker) Update(detected []DetectedEntity) {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()
	seen := make(map[string]bool)

	// First pass: try to match detected entities with existing tracked entities
	for _, d := range detected {
		key := t.entityKey(d)
		seen[key] = true

		if existing, ok := t.entities[key]; ok {
			// Exact match - update position and time
			existing.LastSeen = now
			existing.Entity = d
			t.debugFunc("[Tracker] Exact match: %s at (%d,%d) key=%s clicks=%d",
				d.TemplateName, d.Position.X, d.Position.Y, key, existing.ClickCount)
		} else {
			// No exact match - check if this is an existing entity that moved up
			matchedKey := t.findMovedEntity(d)
			if matchedKey != "" {
				// Found a matching entity that moved - transfer its state
				oldEntity := t.entities[matchedKey]
				t.debugFunc("[Tracker] Moved entity: %s (%d,%d)->(%d,%d) clicks=%d oldKey=%s newKey=%s",
					d.TemplateName, oldEntity.Entity.Position.X, oldEntity.Entity.Position.Y,
					d.Position.X, d.Position.Y, oldEntity.ClickCount, matchedKey, key)
				t.entities[key] = &TrackedEntity{
					Entity:     d,
					ClickCount: oldEntity.ClickCount,
					FirstSeen:  oldEntity.FirstSeen,
					LastSeen:   now,
				}
				// Also transfer blacklist status if applicable
				if _, blacklisted := t.blacklist[matchedKey]; blacklisted {
					t.blacklist[key] = t.blacklist[matchedKey]
					delete(t.blacklist, matchedKey)
					t.debugFunc("[Tracker] Transferred blacklist status to new key")
				}
				delete(t.entities, matchedKey)
				seen[key] = true
			} else {
				// Truly new entity
				t.debugFunc("[Tracker] New entity: %s at (%d,%d) key=%s (existing entities: %d)",
					d.TemplateName, d.Position.X, d.Position.Y, key, len(t.entities))
				t.entities[key] = &TrackedEntity{
					Entity:     d,
					ClickCount: 0,
					FirstSeen:  now,
					LastSeen:   now,
				}
			}
		}
	}

	// Remove expired entities (not seen for TTL)
	for key, tracked := range t.entities {
		if !seen[key] && now.Sub(tracked.LastSeen) > t.ttl {
			t.debugFunc("[Tracker] Expired entity: %s key=%s clicks=%d",
				tracked.Entity.TemplateName, key, tracked.ClickCount)
			delete(t.entities, key)
		}
	}
}

// findMovedEntity checks if a detected entity matches an existing entity that moved up
// Returns the key of the matched entity, or empty string if no match
func (t *EntityTracker) findMovedEntity(d DetectedEntity) string {
	const xThreshold = 30  // X must be within 30px
	const yMaxMove = 200   // Y can move up by at most 200px

	for key, tracked := range t.entities {
		e := tracked.Entity

		// Must be same priority (same template type)
		if e.Priority != d.Priority {
			continue
		}

		// X coordinate must be close
		xDiff := abs(e.Position.X - d.Position.X)
		if xDiff > xThreshold {
			continue
		}

		// Y coordinate: new position should be above (smaller Y) or similar
		// Allow movement up (list scrolling) or small movement down
		yDiff := e.Position.Y - d.Position.Y // positive means moved up
		if yDiff > 0 && yDiff <= yMaxMove {
			// Entity moved up - this is a match
			return key
		}
		if yDiff < 0 && -yDiff <= t.positionThresh {
			// Small movement down - also a match
			return key
		}
	}

	return ""
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// IsBlacklisted checks if an entity is blacklisted
func (t *EntityTracker) IsBlacklisted(e DetectedEntity) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	key := t.entityKey(e)
	_, ok := t.blacklist[key]
	return ok
}

// RecordClick increments click count and blacklists if max reached
// Returns true if blacklisted after this click
func (t *EntityTracker) RecordClick(e DetectedEntity) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	key := t.entityKey(e)

	// Check if already blacklisted
	if _, ok := t.blacklist[key]; ok {
		return true
	}

	// Find or create tracked entity
	tracked, ok := t.entities[key]
	if !ok {
		tracked = &TrackedEntity{
			Entity:     e,
			ClickCount: 0,
			FirstSeen:  time.Now(),
			LastSeen:   time.Now(),
		}
		t.entities[key] = tracked
	}

	tracked.ClickCount++

	// Blacklist if max clicks reached
	if tracked.ClickCount >= t.maxClicks {
		t.blacklist[key] = time.Now()
		return true
	}

	return false
}

// GetClickCount returns the number of clicks for an entity
func (t *EntityTracker) GetClickCount(e DetectedEntity) int {
	t.mu.Lock()
	defer t.mu.Unlock()

	key := t.entityKey(e)
	if tracked, ok := t.entities[key]; ok {
		return tracked.ClickCount
	}
	return 0
}

// FilterBlacklisted returns entities that are not blacklisted
func (t *EntityTracker) FilterBlacklisted(entities []DetectedEntity) []DetectedEntity {
	t.mu.Lock()
	defer t.mu.Unlock()

	var result []DetectedEntity
	for _, e := range entities {
		key := t.entityKey(e)
		if _, blacklisted := t.blacklist[key]; !blacklisted {
			result = append(result, e)
		}
	}
	return result
}

// Reset clears all tracked entities and blacklist (call when entering new game cycle)
func (t *EntityTracker) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.entities = make(map[string]*TrackedEntity)
	t.blacklist = make(map[string]time.Time)
	t.lastHighPriEntity = nil
}

// Stats returns current tracking statistics
func (t *EntityTracker) Stats() (tracked int, blacklisted int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.entities), len(t.blacklist)
}

// SetLastHighPriority records the last clicked high priority entity for ROI optimization
func (t *EntityTracker) SetLastHighPriority(e DetectedEntity) {
	t.mu.Lock()
	defer t.mu.Unlock()
	entityCopy := e
	t.lastHighPriEntity = &entityCopy
}

// GetROI returns a region of interest around the last high priority entity.
// Returns an empty rectangle if no high priority entity has been recorded.
func (t *EntityTracker) GetROI() image.Rectangle {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.lastHighPriEntity == nil {
		return image.Rectangle{}
	}

	e := t.lastHighPriEntity
	margin := t.roiMargin

	// Create ROI around the entity position with margin
	return image.Rectangle{
		Min: image.Point{
			X: e.Position.X - margin,
			Y: e.Position.Y - margin,
		},
		Max: image.Point{
			X: e.Position.X + e.TemplateSize.X + margin,
			Y: e.Position.Y + e.TemplateSize.Y + margin,
		},
	}
}

// HasROI returns true if a ROI has been established
func (t *EntityTracker) HasROI() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.lastHighPriEntity != nil
}

// ExtractPriority extracts the priority number from a filename like "20.png" or "20-1.png"
func ExtractPriority(filename string) int {
	re := regexp.MustCompile(`^(\d+)`)
	matches := re.FindStringSubmatch(filename)
	if len(matches) > 1 {
		if num, err := strconv.Atoi(matches[1]); err == nil {
			return num
		}
	}
	return 0
}

// SortEntitiesByPriority sorts entities by:
// 1. Priority (higher number first)
// 2. Y coordinate (lower on screen first, i.e., higher Y value)
func SortEntitiesByPriority(entities []DetectedEntity) {
	sort.Slice(entities, func(i, j int) bool {
		if entities[i].Priority != entities[j].Priority {
			return entities[i].Priority > entities[j].Priority // Higher priority first
		}
		return entities[i].Position.Y > entities[j].Position.Y // Lower on screen first
	})
}

// Center returns the center point of the entity for clicking
func (e *DetectedEntity) Center() image.Point {
	return image.Point{
		X: e.Position.X + e.TemplateSize.X/2,
		Y: e.Position.Y + e.TemplateSize.Y/2,
	}
}
