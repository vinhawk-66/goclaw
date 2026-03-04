package voicebox

import "context"

func (h *Handler) SendAssistantReply(ctx context.Context, text string, metadata map[string]string) error {
	h.mu.Lock()
	sender := h.ttsSender
	session := h.session
	h.mu.Unlock()
	if sender == nil || session == nil {
		return nil
	}
	emotion := "neutral"
	if metadata != nil && metadata["emotion"] != "" {
		emotion = metadata["emotion"]
	}
	session.Transition(StateSpeaking)
	err := sender.SendResponse(ctx, text, emotion)
	session.Transition(StateIdle)
	return err
}
