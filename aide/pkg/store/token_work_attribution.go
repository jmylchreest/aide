package store

import (
	"encoding/hex"
	"maps"
	"strings"

	"github.com/jmylchreest/aide/aide/pkg/observe"
)

type workHostIdentity struct{ host, session, actor, invocation string }
type workReceiptClaim struct {
	id, tool, hash string
	identity       workHostIdentity
}
type workReceiptMatch struct {
	servers  int
	host     *workReceiptClaim
	conflict bool
}
type workAttribution struct {
	byID       map[string]*workReceiptMatch
	byIdentity map[workHostIdentity]workReceiptClaim
}

func newWorkAttribution() *workAttribution {
	return &workAttribution{byID: make(map[string]*workReceiptMatch), byIdentity: make(map[workHostIdentity]workReceiptClaim)}
}

func workIdentityPart(s string, limit int) bool {
	if len(s) > limit || strings.TrimSpace(s) == "" {
		return false
	}
	for _, r := range s {
		if r < 32 || r == 127 {
			return false
		}
	}
	return true
}
func workDigest(s string) bool {
	if len(s) != 64 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil && s == strings.ToLower(s)
}

func workHostClaim(e *observe.Event) (workReceiptClaim, bool) {
	a := e.Attrs
	identity := workHostIdentity{a["host"], e.SessionID, a["actor_id"], a["invocation_id"]}
	claim := workReceiptClaim{a["work_id"], e.Name, a["work_text_sha256"], identity}
	valid := e.Kind == observe.KindToolCall && a["accounting_version"] == "1" && a["observation_stage"] == "host_result" && a["work_receipt_version"] == "1" && workIdentityPart(claim.id, 128) && workDigest(claim.hash) && workIdentityPart(claim.tool, 256) && workIdentityPart(identity.host, 256) && workIdentityPart(identity.session, 1024) && workIdentityPart(identity.actor, 1024) && workIdentityPart(identity.invocation, 1024)
	return claim, valid
}

// Collect all retained receipts before selecting dates. A later host observation
// can identify earlier server work; conflicting evidence must not disappear
// when a report is narrowed. Each BoltStore is already project-isolated.
// Memory is O(retained receipt IDs and complete host identities); there is no
// silent clipping that could discard contradictory evidence.
func (a *workAttribution) observe(e *observe.Event) {
	attrs := e.Attrs
	if e.Kind != observe.KindToolCall || attrs["accounting_version"] != "1" || !workIdentityPart(attrs["work_id"], 128) {
		return
	}
	id := attrs["work_id"]
	m := a.byID[id]
	if m == nil {
		m = &workReceiptMatch{}
		a.byID[id] = m
	}
	if attrs["observation_stage"] == "server_result" {
		// Even malformed duplicate server receipts make this ID ambiguous.
		m.servers++
		return
	}
	claim, ok := workHostClaim(e)
	if !ok {
		return
	}
	identity := claim.identity
	if previous, ok := a.byIdentity[identity]; ok && previous != claim {
		m.conflict = true
		a.byID[previous.id].conflict = true
	} else {
		a.byIdentity[identity] = claim
	}
	if m.host != nil && *m.host != claim {
		m.conflict = true
	} else {
		m.host = &claim
	}
}

// Attribution never changes the stored event. Legacy direct session evidence
// is retained; each report still keeps host/server text boundaries separate.
func (a *workAttribution) session(e *observe.Event) string {
	m := a.byID[e.Attrs["work_id"]]
	if m == nil {
		return e.SessionID
	}
	if m.servers != 1 || m.conflict {
		return ""
	}
	if m.host == nil {
		return e.SessionID
	}
	h := m.host
	if e.Attrs["work_version"] != "1" || !workDigest(e.Attrs["work_text_sha256"]) || h.tool != e.Name || h.hash != e.Attrs["work_text_sha256"] {
		return e.SessionID
	}
	if e.SessionID != "" && e.SessionID != h.identity.session {
		return ""
	}
	return h.identity.session
}

func (a *workAttribution) project(e *observe.Event) *observe.Event {
	if e.Kind != observe.KindToolCall || e.Attrs["accounting_version"] != "1" || e.Attrs["observation_stage"] != "server_result" {
		return e
	}
	projected := *e
	projected.SessionID = a.session(e)
	m := a.byID[e.Attrs["work_id"]]
	if m != nil && m.servers == 1 && !m.conflict && m.host != nil && projected.SessionID != "" && e.Attrs["work_version"] == "1" && m.host.tool == e.Name && m.host.hash == e.Attrs["work_text_sha256"] {
		projected.Attrs = maps.Clone(e.Attrs)
		projected.Attrs["host"] = m.host.identity.host
		projected.Attrs["actor_id"] = m.host.identity.actor
		projected.Attrs["invocation_id"] = m.host.identity.invocation
		projected.Attrs["work_attribution"] = "receipt"
	}
	return &projected
}
