package voicebox

import (
	"sync"

	"github.com/nextlevelbuilder/goclaw/internal/tts"
)

var (
	ttsManagerMu sync.RWMutex
	ttsManager   *tts.Manager
)

// SetTTSManager wires the gateway-level TTS manager into voicebox channels.
func SetTTSManager(mgr *tts.Manager) {
	ttsManagerMu.Lock()
	ttsManager = mgr
	ttsManagerMu.Unlock()
}

func currentTTSManager() *tts.Manager {
	ttsManagerMu.RLock()
	defer ttsManagerMu.RUnlock()
	return ttsManager
}
