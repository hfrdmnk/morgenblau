package discovermemo

// Invalidator is the payload-agnostic slice of Cache: neither method mentions T, so caches of different payload types satisfy it alike.
type Invalidator interface {
	Invalidate(did string)
	InvalidateAll()
}

// Group fans one invalidation across every payload memo, since a single user action (a hide, a follow, a subscription) stales the sources list and the people list together.
type Group struct {
	caches []Invalidator
}

func NewGroup(caches ...Invalidator) *Group {
	return &Group{caches: caches}
}

func (g *Group) Invalidate(did string) {
	for _, c := range g.caches {
		c.Invalidate(did)
	}
}

func (g *Group) InvalidateAll() {
	for _, c := range g.caches {
		c.InvalidateAll()
	}
}
