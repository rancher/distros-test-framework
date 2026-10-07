package releasebot

import (
	"sync"
)

// Capacity counts the builds each controller runs for the bot, across all runs. A build in
// unknown state keeps its slot (abandoned) until its run sees it finish or stops watching it.
type Capacity struct {
	mu        sync.Mutex
	used      map[string]int
	abandoned map[string]map[string]int // owner -> controller -> slots
}

// take holds a slot whatever the limit: a build already on jenkins (resumed after a restart).
func (c *Capacity) take(controller string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.used == nil {
		c.used = map[string]int{}
	}
	c.used[controller]++
}

// acquire takes a slot on controller if fewer than limit are in use.
func (c *Capacity) acquire(controller string, limit int) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.used == nil {
		c.used = map[string]int{}
	}
	if c.used[controller] >= limit {
		return false
	}

	c.used[controller]++

	return true
}

func (c *Capacity) release(controller string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.used[controller] > 0 {
		c.used[controller]--
	}
}

// abandon marks one of owner's slots on controller as held by a build in unknown state.
func (c *Capacity) abandon(controller, owner string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.abandoned == nil {
		c.abandoned = map[string]map[string]int{}
	}
	if c.abandoned[owner] == nil {
		c.abandoned[owner] = map[string]int{}
	}
	c.abandoned[owner][controller]++
}

// Abandoned reports how many of controller's slots are held by builds in unknown state, all runs.
func (c *Capacity) Abandoned(controller string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, byCtrl := range c.abandoned {
		n += byCtrl[controller]
	}

	return n
}

// releaseAbandoned frees one slot owner's unknown build held on controller (it finished, or the run
// stopped watching it).
func (c *Capacity) releaseAbandoned(controller, owner string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.abandoned[owner][controller] > 0 {
		c.abandoned[owner][controller]--
	}
	if c.used[controller] > 0 {
		c.used[controller]--
	}
}

// InUse reports how many slots are taken on controller.
func (c *Capacity) InUse(controller string) int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.used[controller]
}
