package tlwtag

import "tlog.app/go/tlog"

const (
	_ = tlog.SemanticCommunityBase + iota
	// ...

	SemanticUserBase = tlog.SemanticCommunityBase + 20
)
