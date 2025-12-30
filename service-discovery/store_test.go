package servicediscovery

import (
	"testing"
	"time"
)

// TestStore_Upsert_And_Get verifies that instances can be added and retrieved.
// It checks if the details are preserved correctly.
func TestStore_Upsert_And_Get(t *testing.T) {
	// Setup
	selfID := int64(100)
	store := newStore("10.0.0.1:8080", selfID, OrderingLastSeen)

	// Instance to add
	inst := Instance{
		ID:              200,
		Key:             "key-200",
		Type:            "auth-service",
		Version:         "1.0.0",
		PrivateEndPoint: "10.0.0.2:8080",
		PublicEndPoint:  "public.example.com",
		LastSeenUTC:     time.Now().UTC(),
	}

	// Action
	store.upsert(inst)

	// Assert
	instances := store.GetServiceInstances("auth-service")
	if len(instances) != 1 {
		t.Fatalf("Expected 1 instance, got %d", len(instances))
	}

	got := instances[0]
	if got.ID != inst.ID {
		t.Errorf("Expected ID %d, got %d", inst.ID, got.ID)
	}
	if got.PrivateEndPoint != inst.PrivateEndPoint {
		t.Errorf("Expected PrivateEndPoint %s, got %s", inst.PrivateEndPoint, got.PrivateEndPoint)
	}
}

// TestStore_SelfExclusion verifies that the store does not return the instance
// that corresponds to the 'self' identity of the discovery agent.
func TestStore_SelfExclusion(t *testing.T) {
	// Setup
	selfID := int64(100)
	selfEP := "10.0.0.1:8080"
	store := newStore(selfEP, selfID, OrderingLastSeen)

	// Add 'self' instance (simulating receiving own join message)
	selfInst := Instance{
		ID:              selfID,
		Type:            "auth-service",
		PrivateEndPoint: selfEP,
		LastSeenUTC:     time.Now().UTC(),
	}
	store.upsert(selfInst)

	// Add another instance
	otherInst := Instance{
		ID:              200,
		Type:            "auth-service",
		PrivateEndPoint: "10.0.0.2:8080",
		LastSeenUTC:     time.Now().UTC(),
	}
	store.upsert(otherInst)

	// Action
	instances := store.GetServiceInstances("auth-service")

	// Assert
	if len(instances) != 1 {
		t.Fatalf("Expected 1 instance (excluding self), got %d", len(instances))
	}
	if instances[0].ID == selfID {
		t.Error("Own instance was returned by GetServiceInstances")
	}
}

// TestStore_EndpointCollision verifies that if a new instance claims an existing endpoint,
// the old instance mapping is removed to prevent duplicates or stale data.
func TestStore_EndpointCollision(t *testing.T) {
	store := newStore("self:80", 1, OrderingLastSeen)
	endpoint := "10.0.0.5:8080"

	// Old instance at endpoint
	instA := Instance{
		ID:              10,
		Type:            "payment-service",
		PrivateEndPoint: endpoint,
		LastSeenUTC:     time.Now().UTC(),
	}
	store.upsert(instA)

	// New instance at SAME endpoint (e.g. restart with new ID)
	instB := Instance{
		ID:              11,
		Type:            "payment-service",
		PrivateEndPoint: endpoint,
		LastSeenUTC:     time.Now().UTC(),
	}
	store.upsert(instB)

	// Action
	instances := store.GetServiceInstances("payment-service")

	// Assert
	if len(instances) != 1 {
		t.Fatalf("Expected 1 instance, got %d", len(instances))
	}
	if instances[0].ID != 11 {
		t.Errorf("Expected Instance B (ID 11), got %d", instances[0].ID)
	}
}

// TestStore_SweepExpired verifies that instances older than the timeout are removed.
func TestStore_SweepExpired(t *testing.T) {
	store := newStore("self:80", 1, OrderingLastSeen)
	now := time.Now().UTC()
	expiry := 10 * time.Second

	// Fresh instance
	fresh := Instance{
		ID:              10,
		Type:            "svc",
		PrivateEndPoint: "10.0.0.1:80",
		LastSeenUTC:     now,
	}
	// Expired instance (20 seconds ago)
	expired := Instance{
		ID:              20,
		Type:            "svc",
		PrivateEndPoint: "10.0.0.2:80",
		LastSeenUTC:     now.Add(-20 * time.Second),
	}

	store.upsert(fresh)
	store.upsert(expired)

	// Action
	store.sweepExpired(now, expiry)

	// Assert
	instances := store.GetServiceInstances("svc")
	if len(instances) != 1 {
		t.Fatalf("Expected 1 instance, got %d", len(instances))
	}
	if instances[0].ID != 10 {
		t.Errorf("Expected fresh instance (ID 10) to remain, got %d", instances[0].ID)
	}
}

// TestStore_Ordering_LastSeen verifies that instances are returned sorted by LastSeenUTC descending.
func TestStore_Ordering_LastSeen(t *testing.T) {
	store := newStore("self:80", 1, OrderingLastSeen)
	baseTime := time.Now().UTC()

	// Insert in mixed order
	i1 := Instance{ID: 10, Type: "svc", PrivateEndPoint: "1.1.1.1", LastSeenUTC: baseTime.Add(-10 * time.Minute)} // Oldest
	i2 := Instance{ID: 20, Type: "svc", PrivateEndPoint: "1.1.1.2", LastSeenUTC: baseTime}                        // Newest
	i3 := Instance{ID: 30, Type: "svc", PrivateEndPoint: "1.1.1.3", LastSeenUTC: baseTime.Add(-5 * time.Minute)}  // Middle

	store.upsert(i1)
	store.upsert(i2)
	store.upsert(i3)

	// Action
	list := store.GetServiceInstances("svc")

	// Assert: Expect i2 (newest), i3, i1 (oldest)
	if len(list) != 3 {
		t.Fatalf("Expected 3 instances, got %d", len(list))
	}
	if list[0].ID != 20 || list[1].ID != 30 || list[2].ID != 10 {
		t.Errorf("Order mismatch. Got IDs: %v", getInstanceIDs(list))
	}
}

// TestStore_Ordering_LatestVersion verifies that instances are prioritized by semantic version.
// Higher versions should come first.
func TestStore_Ordering_LatestVersion(t *testing.T) {
	store := newStore("self:80", 1, OrderingLatestVersion)

	// v2.0.0, v1.0.0, v1.1.0
	// Expected Order: v2.0.0, v1.1.0, v1.0.0
	// But note discovery.go implementation:
	// "within the best version, order by last-seen desc"
	// Actually looking at discovery.go:
	// It finds "best" version, filters ONLY those, and returns them.
	// Wait, code says:
	// 		best := instances[0].Version
	// 		... find max version ...
	// 		filtered = append(filtered, inst) if version == best
	// 		return filtered
	// So it ONLY returns the instances with the HIGHEST version.

	i1 := Instance{ID: 1, Type: "svc", PrivateEndPoint: "1", Version: "1.0.0"}
	i2 := Instance{ID: 2, Type: "svc", PrivateEndPoint: "2", Version: "2.0.0"}
	i3 := Instance{ID: 3, Type: "svc", PrivateEndPoint: "3", Version: "1.5.0"}

	store.upsert(i1)
	store.upsert(i2)
	store.upsert(i3)

	// Action
	list := store.GetServiceInstances("svc")

	// Assert
	// Should only contain i2 (Version 2.0.0)
	if len(list) != 1 {
		t.Fatalf("Expected 1 instance (best version), got %d", len(list))
	}
	if list[0].Version != "2.0.0" {
		t.Errorf("Expected version 2.0.0, got %s", list[0].Version)
	}
}

// TestStore_Ordering_RoundRobin verifies that the order rotates with each call.
func TestStore_Ordering_RoundRobin(t *testing.T) {
	store := newStore("self:80", 1, OrderingRoundRobin)

	// Two instances
	i1 := Instance{ID: 10, Type: "svc", PrivateEndPoint: "1", LastSeenUTC: time.Now()}
	i2 := Instance{ID: 20, Type: "svc", PrivateEndPoint: "2", LastSeenUTC: time.Now()}

	store.upsert(i1)
	store.upsert(i2)

	// The implementation sorts by ID asc first: [10, 20]
	// Call 1: Rotate by 1 -> [20, 10]
	list1 := store.GetServiceInstances("svc")
	if len(list1) != 2 {
		t.Fatal("Expected 2 instances")
	}

	// Call 2: Rotate by 2 -> [10, 20] (back to original because mod 2)
	// Oops, let's trace:
	// off := cur.Add(1) % n
	// 1st call: Add(1) -> 1. 1%2 = 1. off=1.
	// rotated = append(inst[1:], inst[:1]...) -> [20, 10]

	// 2nd call: Add(1) -> 2. 2%2 = 0. off=0.
	// rotated = append(inst[0:], inst[:0]...) -> [10, 20]

	list2 := store.GetServiceInstances("svc")

	if list1[0].ID == list2[0].ID {
		t.Error("Round robin did not rotate the list")
	}

	if list1[0].ID != 20 || list2[0].ID != 10 {
		t.Errorf("Unexpected rotation sequence. Call1: %v, Call2: %v", getInstanceIDs(list1), getInstanceIDs(list2))
	}
}

// Helper to extract IDs for logging
func getInstanceIDs(instances []Instance) []int64 {
	ids := make([]int64, len(instances))
	for i, inst := range instances {
		ids[i] = inst.ID
	}
	return ids
}
