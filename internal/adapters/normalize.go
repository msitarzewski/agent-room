package adapters

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/msitarzewski/agent-room/internal/domain"
)

type Context struct {
	ProjectID      string
	ActorID        string
	AgentID        string
	RunID          string
	SessionID      string
	SourceSystem   string
	NativeEventID  string
	SourceSequence int64
	Now            func() time.Time
	NewID          func() string
}

func (c Context) Event(eventType string, subjectType domain.ResourceType, subjectID string, payload any, occurredAt time.Time) (domain.Event, error) {
	if c.ProjectID == "" || c.ActorID == "" {
		return domain.Event{}, fmt.Errorf("adapter project and actor are required")
	}
	if occurredAt.IsZero() {
		if c.Now != nil {
			occurredAt = c.Now()
		} else {
			occurredAt = time.Now().UTC()
		}
	}
	id := ""
	if c.NewID != nil {
		id = c.NewID()
	} else {
		id = randomID()
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return domain.Event{}, err
	}
	event := domain.Event{ID: id, ProjectID: c.ProjectID, Type: eventType, SubjectType: subjectType, SubjectID: subjectID, ActorID: c.ActorID, OccurredAt: occurredAt.UTC(), SchemaVersion: 1, Payload: raw, SourceSystem: c.SourceSystem, SourceEventID: c.NativeEventID, SourceSequence: c.SourceSequence}
	if event.SourceSystem != "" && event.SourceEventID == "" {
		event.SourceEventID = StableSourceID(event.SourceSystem, eventType, subjectID, string(raw))
	}
	return event, nil
}

func StableSourceID(parts ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(digest[:])
}

func randomID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(value[:])
}
