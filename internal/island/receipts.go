package island

// Receipts: what became of a plugin's card. A plugin pushes a card, but Ember
// decides when it shows (a full-screen window, being away or a key's OSD hold
// it back, a card in front of it makes it wait). So Ember says what really
// happened: OnSeen when the card is first drawn on screen, OnDrop (with why)
// when it ends without ever having been drawn. The same facts go to the
// private usage log as one "card" line when the card ends.

// noteCards runs for each frame (drawn) and each timer wake (not drawn). It
// marks the top card as seen when this frame draws it, and otherwise notes
// why it is being held, so an expiry later can say why.
func (u *UI) noteCards(v view, drawn bool) {
	c := u.top()
	if c == nil || !c.seenAt.IsZero() {
		return
	}
	switch {
	case u.mind.away:
		c.heldWhy = "away"
	case u.full:
		c.heldWhy = "fullscreen"
		if u.osdLive() != nil {
			c.heldWhy = "osd"
		}
	case drawn && v == vCard:
		c.seenAt = u.now()
		if c.OnSeen != nil {
			c.OnSeen()
		}
	}
}

// cardEnds records that c left the list (how: expired | removed).
// What it came to is told by flushEnded, after the caller has had its say
// (a button's OnPick runs after the card is removed).
func (u *UI) cardEnds(c *Card, how string) {
	if c.endAt.IsZero() {
		c.endAt, c.endHow = u.now(), how
		u.ended = append(u.ended, c)
	}
}

// flushEnded tells plugins about cards that ended unseen and logs every card's end.
func (u *UI) flushEnded() {
	for _, c := range u.ended {
		seen := !c.seenAt.IsZero()
		if !seen && c.OnDrop != nil {
			why := c.heldWhy
			if c.endHow != "expired" {
				why = "replaced" // the plugin took it back (or the adapter's queue made room) before it was drawn
			} else if why == "" {
				why = "unseen"
			}
			c.OnDrop(why)
		}
		res := c.endHow
		if c.acted != "" {
			res = "acted"
		}
		kv := []any{"id", c.ID, "res", res}
		if seen {
			kv = append(kv, "visible_ms", c.endAt.Sub(c.seenAt).Milliseconds())
		} else {
			kv = append(kv, "unseen", true)
		}
		u.Log("card", kv...)
	}
	u.ended = u.ended[:0]
}
