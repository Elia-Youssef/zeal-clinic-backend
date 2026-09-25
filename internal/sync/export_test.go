package sync

// Hooks for the external tests of this package (package sync_test), which
// need the migrations and therefore cannot live inside package sync.
var (
	EnrichBatch       = enrichBatch
	ReadForeignKeys   = readForeignKeys
	MarkSyncRecovered = markSyncRecovered
)

// PeerStreams is the number of open peer event streams.
func PeerStreams() int {
	peers.mu.RLock()
	defer peers.mu.RUnlock()
	return len(peers.chs)
}
