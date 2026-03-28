package handler

import (
	"context"

	"github.com/danielgtaylor/huma/v2"
)

// MessageItem is the JSON representation of a message.
type MessageItem struct {
	ID      uint64 `json:"id"`
	From    string `json:"from"`
	To      string `json:"to"`
	Content string `json:"content"`
	Type    string `json:"type"`
}

// ListMessagesOutput is the response body for APIListMessages.
type ListMessagesOutput struct {
	Body struct {
		Messages []MessageItem `json:"messages"`
	}
}

// APIListMessages returns messages for an instance as JSON.
func (h *Handler) APIListMessages(ctx context.Context, input *struct {
	Project string `path:"project"`
	Agent   string `query:"agent"`
}) (*ListMessagesOutput, error) {
	inst := h.findInstance(input.Project)
	if inst == nil {
		return nil, huma.Error404NotFound("instance not found")
	}
	s := inst.Store()
	if s == nil {
		return nil, huma.Error503ServiceUnavailable("instance not connected")
	}

	messages, err := s.GetMessages(input.Agent)
	if err != nil {
		return nil, err
	}

	out := &ListMessagesOutput{}
	for _, m := range messages {
		out.Body.Messages = append(out.Body.Messages, MessageItem{
			ID:      m.ID,
			From:    m.From,
			To:      m.To,
			Content: m.Content,
			Type:    m.Type,
		})
	}
	return out, nil
}
